package persistence

// scan.go 内容仓库启动同步 —— 把磁盘实体纳入句柄映射体系
//
// 设计约束：
//   - 扫描 documents / audios / multimedia 及其全部子目录；
//   - 媒体类（图片 / 视频 / 音频）记录 sha16 内容哈希，文本类不记录哈希；
//   - 源文件丢失或损坏的句柄在同步时清理，目录中不留失效映射；
//   - 不自动删除任何实体文件：句柄清理只删目录行，实体由调用方显式操作。

import (
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// storageScanMu 串行化启动同步，避免并发初始化重复建行
var storageScanMu sync.Mutex

// SyncStorageHandles 扫描内容仓库并把全部实体纳入句柄映射（幂等）
//
// 启动时由 InitArchiveDB 自动调用一次；外部也可在批量落盘后手动触发。
func SyncStorageHandles() error {
	if err := EnsureArchiveInitialized(); err != nil {
		return err
	}
	storageScanMu.Lock()
	defer storageScanMu.Unlock()
	return syncStorageHandles()
}

// syncStorageHandles 执行一次完整对齐（调用方须持有 storageScanMu）
func syncStorageHandles() error {
	localDir := filepath.Clean(config.LocalDir)
	present := map[string]bool{}
	scanned, cleaned, skippedDerived := 0, 0, 0

	for _, root := range StorageRoots {
		rootAbs := filepath.Join(localDir, root)
		walkErr := filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, err error) error {
			// 根目录不存在或个别条目不可读：跳过，不阻断其它根
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(localDir, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			// 派生缓存（multimedia/vision、multimedia/footage）不入句柄索引：
			// 它们可随时重建，且与源文件同哈希时会被去重当成重复实体删掉。
			// 不记入 present，历史遗留的目录行会在下面的清理段一并移除（只删行，不删文件）。
			if IsDerivedStoragePath(rel) {
				skippedDerived++
				return nil
			}
			present[rel] = true
			if syncErr := syncFileHandle(rel, path); syncErr != nil {
				logger.SubWarn("Persistence", "Scan", "扫描登记失败(%s): %v", rel, syncErr)
				// 文件不可读 / 图片损坏：清掉既有句柄，避免失效映射滞留
				if delErr := ArchiveDatabase.DeleteResourcesByBlobPath(rel); delErr == nil {
					cleaned++
				}
				return nil
			}
			scanned++
			return nil
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			logger.SubWarn("Persistence", "Scan", "扫描目录失败(%s): %v", root, walkErr)
		}
	}

	// 清理「目录行仍在、实体已丢失」的句柄（仅限三棵内容仓库树）
	rows, err := ArchiveDatabase.ListAllResources()
	if err != nil {
		return err
	}
	for _, res := range rows {
		if !IsStoragePath(res.BlobPath) {
			continue
		}
		if present[res.BlobPath] {
			continue
		}
		if err := ArchiveDatabase.DeleteResource(res.ID); err != nil {
			logger.SubWarn("Persistence", "Scan", "清理失效句柄失败(%s): %v", res.Handle, err)
			continue
		}
		cleaned++
	}

	// 收敛扫描后仍可能存在的重复哈希（历史目录行 / 并发写入），
	// 保留生成目录与集合子目录，舍弃根目录与派生缓存中的重复件
	if removed, err := ArchiveDatabase.DedupeDuplicateHashes(); err != nil {
		logger.SubWarn("Persistence", "Scan", "内容哈希去重失败: %v", err)
	} else if removed > 0 {
		cleaned += removed
	}

	logger.Info("Persistence", "内容仓库句柄同步完成：登记 %d 个实体，清理 %d 个失效/重复项，跳过 %d 个派生缓存文件（不入索引）",
		scanned, cleaned, skippedDerived)
	return nil
}

// syncFileHandle 登记 / 刷新单个实体对应的句柄
func syncFileHandle(rel, abs string) error {
	ext := strings.ToLower(filepath.Ext(rel))
	kind := KindOfExt(ext)
	if kind == "" {
		// 未知类型按文档兜底：扫描要求「全部纳入」，不能因为扩展名不在白名单就漏掉
		kind = KindDocument
	}

	var hash string
	var size int64
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	size = info.Size()

	if IsMediaExtension(ext) {
		hash, size, err = fileSHA16(abs)
		if err != nil {
			return err
		}
		// 图片再做一次头部可解析性校验（SVG 是文本格式，跳过）
		if kind == KindImage && ext != ".svg" {
			if _, _, dimErr := imageDimensions(abs); dimErr != nil {
				return dimErr
			}
		}
	}

	// 内容哈希唯一：扫描到同哈希的第二份实体时，按路径优先级决定保留哪一份。
	// 生成目录/集合子目录 > audios|multimedia 根目录；vision/footage 派生缓存不参与索引。
	if IsMediaExtension(ext) && hash != "" {
		if existing, ok := FindActiveResourceByHash(kind, hash); ok && existing.BlobPath != rel {
			if preferNewPath(rel, existing.BlobPath) {
				// 新实体更优：删除旧实体并把句柄迁移过来
				oldAbs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(existing.BlobPath))
				if oldAbs != abs {
					_ = os.Remove(oldAbs)
				}
				if err := ArchiveDatabase.UpdateResourceBlobPath(existing.ID, rel); err != nil {
					return err
				}
				return ArchiveDatabase.UpdateResourceScan(existing.ID, filepath.Base(rel), ext, resolveMIME("", ext), hash, size)
			}
			// 既有实体更优（或同级）：舍弃扫描到的重复文件
			_ = os.Remove(abs)
			return nil
		}
	}

	existing, found := ArchiveDatabase.FindResourceByBlobPath(rel)
	if found {
		// 类型码变化（例如 .txt 改成 .png）意味着句柄语义已变：旧句柄必须作废
		if existing.Kind != kind {
			if err := ArchiveDatabase.DeleteResource(existing.ID); err != nil {
				return err
			}
		} else {
			return ArchiveDatabase.UpdateResourceScan(existing.ID, filepath.Base(rel), ext, resolveMIME("", ext), hash, size)
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
		Origin:      OriginStorageScan,
		Retain:      RetainNormal,
		State:       StateActive,
	}
	if res.Name == "" {
		res.Name = res.Title
	}
	return ArchiveDatabase.InsertResource(res)
}

// IsStoragePath 判断相对路径是否位于三棵内容仓库树内
func IsStoragePath(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	for _, root := range StorageRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}

// IsDerivedStoragePath 判断相对路径是否位于派生缓存子树（不参与句柄索引）
func IsDerivedStoragePath(rel string) bool {
	rel = normalizeRel(rel)
	for _, root := range DerivedStorageRoots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}
