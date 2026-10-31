package resource

// admit_blob.go 归档副本落点：内容寻址位置、文件名规整、零拷贝硬链接与复制。
//
// 源文件一律原地保留 —— 任何分支都不得搬走或删除用户/前端的真实文件。

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
)

// 归档私有副本的落点：**直接进两棵树的根目录**，与前端写入件同目录、同命名方案（sha16）。
//
// 归属不靠路径区分，而是资源目录里的 blob_owned 列：
//   blob_owned = 1 → 归档自己写入的副本，配额回收可清理
//   blob_owned = 0 → 前端/引擎写入的实体，回收器不得删除
//
// 为什么不再用 archive/ 子目录：路径前缀只是表达归属的一种手段，却给目录结构加了一层
// 无谓嵌套、并让「文档的家」看起来又被分成两半。放进根目录后结构更直白，代价是删除前
// 必须查一次归属与引用计数（见 Delete）——这个代价很小，
// 而且第 2 项守卫（引用计数）本来就必须查。

// snapshotRel 生成内容仓库的相对路径
//
// 媒体类：图片/视频/音频统一落在 multimedia 与 audios 两棵内容树内，供视觉读取端点
// 与本地观看管线按路径取用；命名沿用 sha16，保持内容寻址。
// 文本类：落在 documents 根，**直接使用原文件名**，便于人机两端按名称传递与讨论；
// 同名文件视为同一份文档并被覆盖更新。
// 不可读二进制（压缩包 / 可执行 / 模型 / 未知扩展名）：落在 unknown 根，同样使用原文件名。
func snapshotRel(hash, ext, name string) string {
	if IsMediaExtension(ext) {
		// 音频进 audios/，图片与视频进 multimedia/：两条内容寻址树各司其职
		if KindOfExt(ext) == KindAudio {
			return path.Join(StorageRootAudios, hash+ext)
		}
		return path.Join(StorageRootMultimedia, hash+ext)
	}
	base := safeDocumentName(name, hash, ext)
	if isBinaryStorageExt(ext) {
		return path.Join(StorageRootUnknown, base)
	}
	return path.Join(StorageRootDocuments, base)
}

// safeDocumentName 把建议文件名规整为 documents/ 下可用的裸文件名
//
// 规则：去路径、去 Windows 非法字符、保留中文与空格；无扩展名时补来源扩展名；
// 规整后为空则退回 sha16 命名，保证始终有可落盘的名字。
func safeDocumentName(name, hash, ext string) string {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return hash + ext
	}
	cleaned := make([]rune, 0, len(base))
	for _, r := range base {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			cleaned = append(cleaned, '_')
		default:
			if r < 32 {
				cleaned = append(cleaned, '_')
			} else {
				cleaned = append(cleaned, r)
			}
		}
	}
	base = strings.TrimSpace(string(cleaned))
	// Windows 不允许文件名以空格或点结尾
	base = strings.TrimRight(base, " .")
	if base == "" || base == "." || base == ".." {
		return hash + ext
	}
	if filepath.Ext(base) == "" && ext != "" {
		base += ext
	}
	return base
}

// placeBlob 确定资源的实体位置，返回相对 LocalDir 的路径与「实体是否归归档所有」
//
// **源文件一律原地保留**（硬约束）：库里出现过「导入一份配置文件后原文件消失」的问题，
// 根因就是早期实现用 moveFile 把用户/前端的文件搬走甚至删掉了。
//
// 两条分支：
//  1. 实体已在系统自有的两棵树内（前端写入件、引擎产出）→ **直接引用，不复制不搬移**，
//     并标记 blob_owned=false：这类实体归前端/引擎所有，回收器永远不能删。
//  2. 实体在系统存储之外（工作区文件、下载件、字节流中转件）→ 复制/硬链接一份到
//     snapshotRel 指定的位置，标记 blob_owned=true（可回收；仅系统自建的临时来源会被清理）。
//
// 文本类目标已存在时按「同名即同一份文档」覆盖写入，并保守标记为非归档所有，
// 避免回收器删掉用户可见的原文件名文件。
//
// 返回值 moved 表示是否新写入了实体；owned 表示该实体是否归归档所有（可回收）。
func placeBlob(localPath, hash, ext, name, sourceKind string) (rel string, moved bool, owned bool, err error) {
	localAbs := filepath.Clean(localPath)
	archiveRel := snapshotRel(hash, ext, name)
	archiveAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(archiveRel))

	// 分支 1：已在系统自有的存储内 → 原地引用（受保护，不归归档所有）
	if rel, ok := relativeToLocalDir(localAbs); ok && IsStoragePath(rel) && fileExists(localAbs) {
		return rel, false, false, nil
	}
	if IsMediaExtension(ext) {
		// 媒体：内容寻址，目标存在即同内容复用
		if fileExists(archiveAbs) {
			removeTemporarySource(localAbs, sourceKind)
			return archiveRel, false, false, nil
		}
	} else if fileExists(archiveAbs) {
		// 文本类：同名即同一份文档，内容变化时覆盖原文件
		if err := copyIntoPlace(localAbs, archiveAbs); err != nil {
			return "", false, false, fmt.Errorf("覆盖文档实体失败: %v", err)
		}
		removeTemporarySource(localAbs, sourceKind)
		return archiveRel, false, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(archiveAbs), 0755); err != nil {
		return "", false, false, fmt.Errorf("创建内容仓库目录失败: %v", err)
	}
	// 优先硬链接：零拷贝且不额外占空间；跨卷或权限不足时退回复制
	if err := os.Link(localAbs, archiveAbs); err == nil {
		removeTemporarySource(localAbs, sourceKind)
		return archiveRel, true, true, nil
	}
	if err := copyIntoPlace(localAbs, archiveAbs); err != nil {
		return "", false, false, fmt.Errorf("写入内容仓库失败: %v", err)
	}
	removeTemporarySource(localAbs, sourceKind)
	return archiveRel, true, true, nil
}

// 归属说明：实体是否可回收记录在资源目录的 blob_owned 列上（见 Resource.BlobOwned），
// 不再用路径前缀表达。删除前仍须复核引用计数（见 Delete），
// 两道守卫合起来保证「回收器不会删掉前端可见的文件」。

// removeTemporarySource 仅清理系统自建的临时来源
//
// 判据是来源类型而非路径：下载件（url）与字节流落地的中转文件（data / bytes）由本进程
// 创建，可以清理；path 与 workspace 指向用户或前端的真实文件，**必须保留**。
func removeTemporarySource(abs, sourceKind string) {
	switch sourceKind {
	case SourceKindURL, SourceKindData, SourceKindBytes:
		_ = os.Remove(abs)
	}
}

// copyIntoPlace 把源文件复制到目标路径（不触碰源文件）
func copyIntoPlace(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return nil
}
