package resource

// register.go 文件实体 ↔ 句柄的同步原语（persistence 落盘观察者的实现）
//
// 新体系下本地资源的身份是 /file/read/<rel>：
//   - 登记 = 确保 url_master 有该 URL 的记录，并给它绑定一个对应类型的句柄；
//   - 同一路径重复登记复用同一句柄，不新增序号；
//   - 实体丢失时删除映射（只删索引，不删文件——文件已由调用方处理）。
//
// 写入 / 删除 / 移动 / 读取等所有入口都调用这里的函数，
// 句柄分配仍由 store.InsertResource 统一完成。

import (
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/multimodal"
)

// RegisterPath 登记内容仓库内的文件并返回最终保留路径与句柄
func RegisterPath(rel string) (canonicalRel string, handle string, err error) {
	rel = normalizeRel(rel)
	if !IsStoragePath(rel) {
		return rel, "", nil
	}
	// 派生缓存（视觉副本 / 视频抽帧）不参与句柄体系
	if IsDerivedStoragePath(rel) {
		return rel, "", nil
	}
	if err := EnsureReady(); err != nil {
		return rel, "", nil
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		return rel, "", statErr
	}
	handle, canonicalRel, err = upsertHandleForPath(rel, abs)
	return canonicalRel, handle, err
}

// UpsertPath 为内容仓库内的文件登记 / 刷新句柄
func UpsertPath(rel string) (string, error) {
	_, handle, err := RegisterPath(rel)
	return handle, err
}

// ForPath 按实体路径查询句柄映射（不触发计算）
func ForPath(rel string) (Resource, bool) {
	rel = normalizeRel(rel)
	if rel == "" || EnsureReady() != nil {
		return Resource{}, false
	}
	return archiveDB.FindResourceByBlobPath(rel)
}

// FindActiveByHash 查找内容哈希命中且实体在位的资源
func FindActiveByHash(kind, hash string) (Resource, bool) {
	if hash == "" || EnsureReady() != nil {
		return Resource{}, false
	}
	existing, ok := archiveDB.FindResourceByHash(kind, hash)
	if !ok {
		return Resource{}, false
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(existing.BlobPath))
	if existing.State != StateActive || !fileExists(abs) {
		_ = archiveDB.DeleteResource(existing.ID)
		return Resource{}, false
	}
	return existing, true
}

// upsertHandleForPath 登记单个实体
func upsertHandleForPath(rel, abs string) (handle string, canonicalRel string, err error) {
	if err := EnsureReady(); err != nil {
		return "", rel, nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", rel, err
	}
	ext := strings.ToLower(filepath.Ext(rel))
	kind := KindOfExt(ext)
	if kind == "" {
		kind = KindDocument
	}
	var hash string
	var size = info.Size()
	if IsMediaExtension(ext) {
		hash, size, err = fileSHA16(abs)
		if err != nil {
			return "", rel, err
		}
		if kind == KindImage {
			// 动图与静图分开：真实字节判定，扩展名只做初筛
			if isAnimatedFile(abs, ext) {
				kind = KindAnimated
			}
		}
	}
	rawURL := NormalizeURL(FileReadURL(rel))
	existing, found := archiveDB.FindResourceByURL(rawURL)
	if found && existing.State == StateActive && fileExists(abs) {
		if existing.Kind != kind {
			// 类型码变化：InsertResource 会作废旧句柄并重绑
			res := Resource{
				Kind: kind, Name: filepath.Base(rel), Ext: ext, MIME: resolveMIME("", ext),
				ContentHash: hash, BlobPath: rel, SizeBytes: size, Title: strings.TrimSuffix(filepath.Base(rel), ext),
				Origin: existing.Origin, Retain: existing.Retain, State: StateActive, BlobOwned: existing.BlobOwned,
			}
			if err := archiveDB.InsertResource(&res); err != nil {
				return "", rel, err
			}
			return res.Handle, rel, nil
		}
		if err := archiveDB.UpdateResourceScan(existing.ID, filepath.Base(rel), ext, resolveMIME("", ext), hash, size); err != nil {
			return "", rel, err
		}
		archiveDB.TouchResource(existing.ID)
		return existing.Handle, rel, nil
	}
	res := &Resource{
		Kind:        kind,
		Name:        filepath.Base(rel),
		Ext:         ext,
		MIME:        resolveMIME("", ext),
		ContentHash: hash,
		BlobPath:    rel,
		SizeBytes:   size,
		Title:       strings.TrimSuffix(filepath.Base(rel), ext),
		Origin:      OriginClientUpload,
		Retain:      RetainNormal,
		State:       StateActive,
	}
	if err := archiveDB.InsertResource(res); err != nil {
		return "", rel, err
	}
	return res.Handle, rel, nil
}

// isAnimatedFile 以字节探针判定图片是否动图（大文件直接按静态图处理，避免整读）
func isAnimatedFile(abs, ext string) bool {
	if !MayBeAnimated(ext) {
		return false
	}
	info, err := os.Stat(abs)
	if err != nil || info.Size() > 50*1024*1024 {
		return ext == ".gif" || ext == ".apng"
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return multimodal.IsAnimatedImage(data)
}

// TouchHandlesForRead 读路径命中时刷新句柄；实体已缺失则清除失效句柄
func TouchHandlesForRead(rel string) {
	rel = normalizeRel(rel)
	if !IsStoragePath(rel) || EnsureReady() != nil {
		return
	}
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(rel))
	if !fileExists(abs) {
		_ = archiveDB.DeleteResourcesByBlobPath(rel)
		return
	}
	if existing, found := archiveDB.FindResourceByBlobPath(rel); found {
		archiveDB.TouchResource(existing.ID)
	}
}

// RemoveHandlesForPath 删除实体对应的全部句柄（文件 / 目录删除）
func RemoveHandlesForPath(rel string) {
	rel = normalizeRel(rel)
	if rel == "" || EnsureReady() != nil {
		return
	}
	_ = archiveDB.DeleteResourcesByBlobPrefix(rel)
}

// RewriteHandlesForMove 把移动前的路径映射改写为移动后的路径
func RewriteHandlesForMove(oldRel, newRel string) {
	oldRel = normalizeRel(oldRel)
	newRel = normalizeRel(newRel)
	if oldRel == "" || newRel == "" || EnsureReady() != nil {
		return
	}
	if !IsStoragePath(newRel) {
		_ = archiveDB.DeleteResourcesByBlobPrefix(oldRel)
		return
	}
	_, _ = archiveDB.RewriteBlobPrefix(oldRel, newRel)
}

// pathPriority 内容仓库路径的保留优先级（值越大越优先保留）
//
//   - 生成目录 / 集合目录等子目录：2
//   - audios / multimedia 根目录：1
//   - 派生缓存：0
func pathPriority(rel string) int {
	rel = normalizeRel(rel)
	if IsDerivedStoragePath(rel) {
		return 0
	}
	if isRootMediaPath(rel) {
		return 1
	}
	return 2
}

// preferNewPath 新路径是否应取代既有路径成为该内容哈希的唯一实体
func preferNewPath(newRel, existingRel string) bool {
	return pathPriority(newRel) > pathPriority(existingRel)
}
