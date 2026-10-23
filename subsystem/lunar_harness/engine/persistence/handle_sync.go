package persistence

// handle_sync.go 文件操作与句柄映射的同步原语
//
// 写入 / 删除 / 移动 / 读取等所有入口在落盘完成后都调用这里的函数，
// 保证「实体在、句柄在；实体变、句柄变；实体丢失、句柄清除」。
// 句柄分配仍由 ArchiveDatabase.InsertResource 统一完成。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"os"
	"path/filepath"
	"strings"
)

// RegisterPath 登记内容仓库内的文件并返回最终保留路径与句柄
//
// 媒体类会执行内容哈希唯一性裁决：若同哈希已存在且既有路径优先级更高，
// 新文件会被删除并返回既有路径；若新路径优先级更高（生成目录 vs 根目录），
// 既有句柄会迁移到新路径并删除根目录旧实体。
func RegisterPath(rel string) (canonicalRel string, handle string, err error) {
	rel = normalizeRel(rel)
	if !IsStoragePath(rel) {
		return rel, "", nil
	}
	// 派生缓存（视觉副本 / 视频切片）不参与句柄体系：可重建、无需引用，
	// 且与源文件同哈希时会被去重逻辑当成「重复实体」误删。
	if IsDerivedStoragePath(rel) {
		return rel, "", nil
	}
	if err := EnsureArchiveInitialized(); err != nil {
		return rel, "", nil
	}
	abs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(rel))
	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		return rel, "", statErr
	}
	// 注意返回顺序：多值赋值按位置绑定，与参数名无关。
	// upsertHandleWithKnownHash 是 (handle, canonicalRel, err)，本函数对外是
	// (canonicalRel, handle, err)，因此左侧必须先写 handle 再写 canonicalRel；
	// 直接 `return upsert...` 会把两者对调，调用方就会把句柄（@I.00x）当成路径
	// 拼进 /file/read/，表现为图片 404。
	handle, canonicalRel, err = upsertHandleWithKnownHash(rel, abs, "")
	return canonicalRel, handle, err
}

// UpsertHandleForPath 为内容仓库内的文件登记 / 刷新句柄（路径不在三棵树内返回空串）
func UpsertHandleForPath(rel string) (string, error) {
	_, handle, err := RegisterPath(rel)
	return handle, err
}

// ResourceForPath 按实体路径查询句柄映射（不触发计算，供列表等只读场景使用）
func ResourceForPath(rel string) (ArchiveResource, bool) {
	rel = normalizeRel(rel)
	if rel == "" || EnsureArchiveInitialized() != nil {
		return ArchiveResource{}, false
	}
	return ArchiveDatabase.FindResourceByBlobPath(rel)
}

// FindActiveResourceByHash 查找内容哈希命中且实体在位的资源（去重写入使用）
func FindActiveResourceByHash(kind, hash string) (ArchiveResource, bool) {
	if hash == "" || EnsureArchiveInitialized() != nil {
		return ArchiveResource{}, false
	}
	existing, ok := ArchiveDatabase.FindResourceByHash(kind, hash)
	if !ok {
		return ArchiveResource{}, false
	}
	// 派生缓存里的历史目录行不算「可复用实体」：只清目录行，绝不把源文件迁移进缓存目录
	if IsDerivedStoragePath(existing.BlobPath) {
		_ = ArchiveDatabase.DeleteResource(existing.ID)
		return ArchiveResource{}, false
	}
	abs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(existing.BlobPath))
	if existing.State != StateActive || !fileExists(abs) {
		// 失效句柄不参与去重，同时清掉
		_ = ArchiveDatabase.DeleteResource(existing.ID)
		return ArchiveResource{}, false
	}
	return existing, true
}

// upsertHandleWithKnownHash 登记句柄；hash 为空时按扩展名决定是否计算
//
// 返回 canonicalRel：内容哈希已存在时，磁盘上最终保留的实体路径。
// 若新路径优先级更高（生成目录 > 根目录 > 派生缓存），会把既有句柄迁移到新路径
// 并删除旧实体；否则删除新写入的重复实体，返回既有路径。由此保证
// 「同一内容哈希在句柄体系中只对应一份实体」。
func upsertHandleWithKnownHash(rel, abs, hash string) (handle string, canonicalRel string, err error) {
	if err := EnsureArchiveInitialized(); err != nil {
		return "", rel, nil
	}
	ext := strings.ToLower(filepath.Ext(rel))
	kind := KindOfExt(ext)
	if kind == "" {
		kind = KindDocument
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", rel, err
	}
	size := info.Size()
	if IsMediaExtension(ext) && hash == "" {
		hash, size, err = fileSHA16(abs)
		if err != nil {
			return "", rel, err
		}
	}

	// 内容哈希唯一：媒体类先处理与既有实体的冲突
	if IsMediaExtension(ext) && hash != "" {
		if existing, ok := FindActiveResourceByHash(kind, hash); ok && existing.BlobPath != rel {
			if preferNewPath(rel, existing.BlobPath) {
				// 新路径更优（如从根目录迁到生成目录）：删除旧实体，句柄改指新路径
				oldAbs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(existing.BlobPath))
				if oldAbs != abs {
					_ = os.Remove(oldAbs)
				}
				if err := ArchiveDatabase.UpdateResourceBlobPath(existing.ID, rel); err != nil {
					return "", rel, err
				}
				if err := ArchiveDatabase.UpdateResourceScan(existing.ID, filepath.Base(rel), ext, resolveMIME("", ext), hash, size); err != nil {
					return "", rel, err
				}
				ArchiveDatabase.TouchResource(existing.ID)
				return existing.Handle, rel, nil
			}
			// 既有路径更优（或同级）：删除刚写入的重复实体，返回既有路径
			_ = os.Remove(abs)
			return existing.Handle, existing.BlobPath, nil
		}
	}

	existing, found := ArchiveDatabase.FindResourceByBlobPath(rel)
	if found {
		// 类型码变了：旧句柄语义已失效，删除后重新分配
		if existing.Kind != kind {
			if err := ArchiveDatabase.DeleteResource(existing.ID); err != nil {
				return "", rel, err
			}
		} else {
			if err := ArchiveDatabase.UpdateResourceScan(existing.ID, filepath.Base(rel), ext, resolveMIME("", ext), hash, size); err != nil {
				return "", rel, err
			}
			ArchiveDatabase.TouchResource(existing.ID)
			return existing.Handle, rel, nil
		}
	}

	res := &ArchiveResource{
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
	if err := ArchiveDatabase.InsertResource(res); err != nil {
		return "", rel, err
	}
	return res.Handle, rel, nil
}

// TouchHandlesForRead 读路径命中时刷新句柄；实体已缺失则清除失效句柄
func TouchHandlesForRead(rel string) {
	rel = normalizeRel(rel)
	if !IsStoragePath(rel) || EnsureArchiveInitialized() != nil {
		return
	}
	abs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(rel))
	if !fileExists(abs) {
		_ = ArchiveDatabase.DeleteResourcesByBlobPath(rel)
		return
	}
	if existing, found := ArchiveDatabase.FindResourceByBlobPath(rel); found {
		ArchiveDatabase.TouchResource(existing.ID)
	}
}

// RemoveHandlesForPath 删除实体对应的全部句柄（用于文件 / 目录删除）
func RemoveHandlesForPath(rel string) {
	rel = normalizeRel(rel)
	if rel == "" || EnsureArchiveInitialized() != nil {
		return
	}
	_ = ArchiveDatabase.DeleteResourcesByBlobPrefix(rel)
}

// RewriteHandlesForMove 把移动前的路径映射改写为移动后的路径
//
// 返回是否仍有句柄落在内容仓库内（调用方据此决定要不要为新位置补登记）。
func RewriteHandlesForMove(oldRel, newRel string) {
	oldRel = normalizeRel(oldRel)
	newRel = normalizeRel(newRel)
	if oldRel == "" || newRel == "" || EnsureArchiveInitialized() != nil {
		return
	}
	// 目标不在内容仓库：句柄体系不再覆盖该实体，直接移除
	if !IsStoragePath(newRel) {
		_ = ArchiveDatabase.DeleteResourcesByBlobPrefix(oldRel)
		return
	}
	_, _ = ArchiveDatabase.RewriteBlobPrefix(oldRel, newRel)
}

// pathPriority 内容仓库路径的保留优先级（值越大越优先保留）
//
//   - 生成目录 / 集合目录等子目录：2（最高）
//   - audios / multimedia 根目录：1（与生成目录同内容时优先舍弃）
//   - 派生缓存（vision / footage）：0（最低，随时可重建；正常不会进入索引）
//
// 文本类不参与哈希去重，优先级不适用于它们。
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

// isRootMediaPath 判断路径是否直接位于 audios/ 或 multimedia/ 根目录
func isRootMediaPath(rel string) bool {
	rel = normalizeRel(rel)
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		return false
	}
	return parts[0] == StorageRootAudios || parts[0] == StorageRootMultimedia
}

// preferNewPath 新路径是否应取代既有路径成为该内容哈希的唯一实体
//
// 相同优先级时保留既有实体（稳定句柄优先）；只有新路径优先级更高时才迁移。
func preferNewPath(newRel, existingRel string) bool {
	return pathPriority(newRel) > pathPriority(existingRel)
}

// normalizeRel 规整相对 LocalDir 的路径为斜杠形式
func normalizeRel(rel string) string {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	rel = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(rel)), "/")
	if rel == "." {
		return ""
	}
	return rel
}
