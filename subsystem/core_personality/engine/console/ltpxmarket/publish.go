package ltpxmarket

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"CorePersonality/engine/console/channel"
)

// Publish 发布 LTPX 包：以磁盘上的 metadata.json 为准 → 合并标签（metadata tags +
// 描述中 #标签）→ zip 打包（含顶层目录 = 包目录名）→ upsert Release（tag = 包 id）→
// 以规范名 ltpx_<id>.ltpx 上传附件。
// description 为用户输入的发布描述（可为空，回退 metadata.json 的 description）。
func Publish(cfg Config, pkg LocalPackage, description string) error {
	if err := validateConfig(cfg, true); err != nil {
		return err
	}
	reportProgress(-1)
	logf("开始发布 LTPX 包 %s（仓库 %s/%s）", orDash(pkg.Meta.ID), cfg.GiteeOwner, cfg.GiteeRepo)

	// ==== 1. 读取元数据（扫描后包内容可能被修改，以磁盘为准） ====
	metaPath := filepath.Join(pkg.Dir, "metadata.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("读取 metadata.json 失败: %w", err)
	}
	var meta PackageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("解析 metadata.json 失败: %w", err)
	}
	if meta.ID == "" {
		return fmt.Errorf("metadata.json 缺少 id 字段（%s）", metaPath)
	}
	dirName := pkg.DirName
	if dirName == "" {
		dirName = filepath.Base(pkg.Dir)
	}

	// ==== 2. 标签合并：metadata.json tags + 描述中 #标签 ====
	tags := mergeTags(meta.Tags, extractHashTags(description))
	logf("  包: %s（%s）  标签: %s", meta.Title, meta.ID, strings.Join(tags, ", "))

	// ==== 3. zip 打包到临时目录（含顶层目录 = 包目录名，落位时随包保留） ====
	outZip := filepath.Join(os.TempDir(), marketAssetName(meta.ID))
	_ = os.Remove(outZip) // 清理可能的残留
	defer os.Remove(outZip)
	total := dirTotalSize(pkg.Dir)
	logf("  打包 %s（%.1f MB）→ %s", dirName, float64(total)/(1024*1024), filepath.Base(outZip))
	if err := zipDir(pkg.Dir, dirName, outZip, total); err != nil {
		return err
	}

	// ==== 4. SHA-256 与 Release 描述 ====
	sum, err := fileSHA256(outZip)
	if err != nil {
		logf("  [WARN] 计算 SHA-256 失败: %v", err)
		sum = ""
	}
	body := buildMarketBody(meta, tags, description, sum)

	// ==== 5. upsert：删旧 Release/tag → 创建新 Release → 上传附件 ====
	reportProgress(60)
	releaseID, err := upsertRelease(cfg, meta.ID, meta.Title, body)
	if err != nil {
		return err
	}
	reportProgress(70)
	logf("  上传附件: %s（%.1f MB）", marketAssetName(meta.ID), float64(fileSize(outZip))/(1024*1024))
	if err := uploadAsset(cfg, releaseID, outZip, marketAssetName(meta.ID), 70, 30); err != nil {
		return fmt.Errorf("上传附件失败: %w", err)
	}

	logf("✓ LTPX 包 %s 发布完成", meta.ID)
	reportProgress(100)
	return nil
}

// marketAssetName 市场附件规范名：ltpx_<id>.ltpx
func marketAssetName(id string) string {
	return assetPrefix + id + assetExt
}

// ReadArchivePackage 读取已打包的 LTPX 安装包（.ltpx/.zip，琉璃导出格式）：
// 从包内定位 metadata.json 并解析（不解压），返回发布候选项
// （DirName/Dir 取自安装包文件，Size 为文件大小）。
func ReadArchivePackage(archivePath string) (LocalPackage, error) {
	if !fileExists(archivePath) {
		return LocalPackage{}, fmt.Errorf("文件不存在: %s", archivePath)
	}
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return LocalPackage{}, fmt.Errorf("无法读取安装包（内容须为 zip 格式的 .ltpx/.zip）: %w", err)
	}
	defer reader.Close()

	// 条目布局兼容两种：<顶层目录>/metadata.json（琉璃导出格式）与根级 metadata.json；
	// 更深层级的同名文件（包内嵌套资源）不误读
	var raw []byte
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		if path.Base(name) != "metadata.json" {
			continue
		}
		dir := path.Dir(name)
		if dir != "." && strings.Contains(dir, "/") {
			continue
		}
		src, oErr := f.Open()
		if oErr != nil {
			return LocalPackage{}, oErr
		}
		raw, err = io.ReadAll(src)
		src.Close()
		if err != nil {
			return LocalPackage{}, err
		}
		break
	}
	if raw == nil {
		return LocalPackage{}, fmt.Errorf("安装包内缺少 metadata.json（不是有效的 LTPX 包）")
	}
	var meta PackageMeta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return LocalPackage{}, fmt.Errorf("解析 metadata.json 失败: %w", err)
	}
	if meta.ID == "" {
		return LocalPackage{}, fmt.Errorf("metadata.json 缺少 id 字段")
	}
	return LocalPackage{
		DirName: filepath.Base(archivePath),
		Dir:     archivePath,
		Meta:    meta,
		Size:    fileSize(archivePath),
	}, nil
}

// PublishArchive 发布已打包的 LTPX 安装包（.ltpx/.zip）：从包内读取 metadata.json
// （不解压）→ 合并标签（metadata tags + 描述中 #标签）→ 计算哈希 → upsert Release →
// 以规范名 ltpx_<id>.ltpx 上传（磁盘文件不必更名）。description 为用户输入的发布
// 描述（可为空，回退 metadata.json 的 description）。
func PublishArchive(cfg Config, archivePath, description string) error {
	if err := validateConfig(cfg, true); err != nil {
		return err
	}
	switch ext := strings.ToLower(filepath.Ext(archivePath)); ext {
	case ".ltpx", ".zip":
		// 琉璃生态标准格式与其 zip 兼容形态均可
	default:
		return fmt.Errorf("不支持的文件类型 %s（仅支持 .ltpx、.zip）", ext)
	}
	reportProgress(-1)
	logf("开始发布安装包 %s（仓库 %s/%s）", filepath.Base(archivePath), cfg.GiteeOwner, cfg.GiteeRepo)

	// ==== 1. 包内读取元数据（不解压） ====
	pkg, err := ReadArchivePackage(archivePath)
	if err != nil {
		return err
	}
	meta := pkg.Meta

	// ==== 2. 标签合并与描述 ====
	tags := mergeTags(meta.Tags, extractHashTags(description))
	logf("  包: %s（%s）  标签: %s", meta.Title, meta.ID, strings.Join(tags, ", "))
	sum, err := fileSHA256(archivePath)
	if err != nil {
		logf("  [WARN] 计算 SHA-256 失败: %v", err)
		sum = ""
	}
	body := buildMarketBody(meta, tags, description, sum)

	// ==== 3. upsert：删旧 Release/tag → 创建新 Release → 上传附件（规范名，原文件不动） ====
	reportProgress(20)
	releaseID, err := upsertRelease(cfg, meta.ID, meta.Title, body)
	if err != nil {
		return err
	}
	reportProgress(30)
	logf("  上传附件: %s（%.1f MB）", marketAssetName(meta.ID), float64(fileSize(archivePath))/(1024*1024))
	if err := uploadAsset(cfg, releaseID, archivePath, marketAssetName(meta.ID), 30, 70); err != nil {
		return fmt.Errorf("上传附件失败: %w", err)
	}

	logf("✓ LTPX 包 %s 发布完成", meta.ID)
	reportProgress(100)
	return nil
}

// buildMarketBody 组装市场 Release 描述（结构化头部 + 描述正文）。
// 描述为空时回退 metadata.json 的 description。
func buildMarketBody(meta PackageMeta, tags []string, description, sha string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", bodyMarker, meta.Title)
	fmt.Fprintf(&b, "ID: %s\n", meta.ID)
	fmt.Fprintf(&b, "Tags: %s\n", strings.Join(tags, ", "))
	if sha != "" {
		fmt.Fprintf(&b, "SHA-256: %s\n", sha)
	}
	b.WriteString("\n")
	if d := strings.TrimSpace(description); d != "" {
		b.WriteString(d)
		b.WriteString("\n")
	} else if meta.Description != "" {
		b.WriteString(meta.Description)
		b.WriteString("\n")
	}
	return b.String()
}

// upsertRelease 单版本 upsert：按 tag（= 包 id）查旧 Release，存在则删 Release 与 tag 后重建。
// 返回新 Release ID（附件上传需要）。
func upsertRelease(cfg Config, tag, name, body string) (int64, error) {
	query := url.Values{}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	var existing giteeRelease
	existPath := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(tag))
	if err := apiRequest(existPath, query, &existing); err == nil && existing.ID > 0 {
		// 通道校验先于删除：若该 tag 属于更新包通道（包 id 与版本号撞车），
		// 删除会连带干掉更新包发布，必须拦下并提示更换包 id
		if kind := releaseKind(existing); kind != channel.Market {
			return 0, channelMismatchError(tag, kind)
		}
		logf("  旧发布已存在（ID=%d），先删除", existing.ID)
		delRelease := fmt.Sprintf("/repos/%s/%s/releases/%d?access_token=%s",
			cfg.GiteeOwner, cfg.GiteeRepo, existing.ID, url.QueryEscape(cfg.GiteeToken))
		if err := deleteJSON(delRelease); err != nil {
			return 0, fmt.Errorf("删除旧 Release 失败: %w", err)
		}
		// 删除 Release 不回收 tag，需单独删除（个别 Gitee 版本连带删除，失败不阻断：
		// tag 已无 Release 挂靠，创建 Release 时可直接复用既有 tag）
		delTag := fmt.Sprintf("/repos/%s/%s/tags/%s?access_token=%s",
			cfg.GiteeOwner, cfg.GiteeRepo, url.PathEscape(tag), url.QueryEscape(cfg.GiteeToken))
		if err := deleteJSON(delTag); err != nil {
			logf("  [WARN] 删除旧 tag 失败（不影响重建，必要时到 Gitee 手动清理）: %v", err)
		}
	}

	payload, _ := json.Marshal(map[string]string{
		"access_token":     cfg.GiteeToken,
		"tag_name":         tag,
		"name":             name,
		"body":             body,
		"target_commitish": defaultBranch(cfg),
	})
	resp, err := postJSON(fmt.Sprintf("/repos/%s/%s/releases", cfg.GiteeOwner, cfg.GiteeRepo), payload)
	if err != nil {
		return 0, fmt.Errorf("创建 Release 失败: %w", err)
	}
	var release giteeRelease
	if err := json.Unmarshal(resp, &release); err != nil {
		return 0, fmt.Errorf("解析 Release 响应失败: %w", err)
	}
	if release.ID == 0 {
		return 0, fmt.Errorf("Release 创建异常: %s", truncate(string(resp), 300))
	}
	logf("  Release 已创建: %s（ID=%d）", tag, release.ID)
	return release.ID, nil
}

// defaultBranch 解析仓库默认分支（创建 Release 的 target_commitish 必填）。
// 优先读仓库信息；个别仓库信息端点不可达时回退到分支列表首项。
func defaultBranch(cfg Config) string {
	query := url.Values{}
	if cfg.GiteeToken != "" {
		query.Set("access_token", cfg.GiteeToken)
	}
	var info struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s", cfg.GiteeOwner, cfg.GiteeRepo), query, &info); err == nil && info.DefaultBranch != "" {
		return info.DefaultBranch
	}
	var branches []struct {
		Name string `json:"name"`
	}
	if err := apiRequest(fmt.Sprintf("/repos/%s/%s/branches", cfg.GiteeOwner, cfg.GiteeRepo), query, &branches); err == nil && len(branches) > 0 {
		return branches[0].Name
	}
	return "master"
}

// zipDir 将 srcDir 打包为 zip，包内以 topDir 为顶层目录（条目形如 topDir/...），
// total>0 时把打包进度映射到 10-55 区间上报。
func zipDir(srcDir, topDir, dest string, total int64) (err error) {
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("创建 zip 文件失败: %w", err)
	}
	defer func() {
		if cErr := out.Close(); err == nil {
			err = cErr
		}
	}()

	w := zip.NewWriter(out)
	var sent int64
	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if info.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(srcDir, path)
		if rErr != nil {
			return rErr
		}
		hdr, hErr := zip.FileInfoHeader(info)
		if hErr != nil {
			return hErr
		}
		hdr.Name = topDir + "/" + filepath.ToSlash(rel)
		hdr.Method = zip.Deflate
		writer, cErr := w.CreateHeader(hdr)
		if cErr != nil {
			return cErr
		}
		src, oErr := os.Open(path)
		if oErr != nil {
			return oErr
		}
		_, cpErr := io.Copy(writer, src)
		clErr := src.Close() // 句柄须先于返回关闭：Windows 下未释放句柄的文件无法删除
		if cpErr != nil {
			return cpErr
		}
		if clErr != nil {
			return clErr
		}
		if total > 0 {
			sent += info.Size()
			reportProgress(10 + int(sent*45/total)) // 打包占整体进度 10-55
		}
		return nil
	})
	if walkErr != nil {
		_ = w.Close()
		return fmt.Errorf("打包失败: %w", walkErr)
	}
	// 中央目录必须在底层文件关闭前写入
	if cErr := w.Close(); cErr != nil {
		return fmt.Errorf("收尾 zip 失败: %w", cErr)
	}
	return nil
}

// fileSHA256 计算文件 SHA-256
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
