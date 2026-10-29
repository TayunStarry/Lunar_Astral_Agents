package resource

// sync.go 内容仓库启动同步 —— 把磁盘实体自动纳入句柄体系
//
// 扫描 documents / audios / multimedia 及其全部子目录：
//   - 图片按真实字节区分静图（I）与动图（G）；
//   - 视频 / 音频 / 文件分别落 V / A / D；
//   - 派生缓存（multimedia/vision、multimedia/footage、audios/cache）不入索引；
//   - 实体丢失的句柄在同步时清理，目录中不留失效映射；
//   - 不自动删除任何实体文件，删除只针对索引行。

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// SyncStorageHandles 扫描内容仓库并把全部实体纳入句柄映射（幂等）
func SyncStorageHandles() error {
	if err := EnsureReady(); err != nil {
		return err
	}
	storageScanMu.Lock()
	defer storageScanMu.Unlock()
	return syncStorageHandles()
}

// syncStorageHandles 执行一次完整对齐
func syncStorageHandles() error {
	localDir := filepath.Clean(config.LocalDir)
	present := map[string]bool{}
	scanned, cleaned, skippedDerived := 0, 0, 0

	for _, root := range StorageRoots {
		rootAbs := filepath.Join(localDir, root)
		walkErr := filepath.WalkDir(rootAbs, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			rel, relErr := filepath.Rel(localDir, path)
			if relErr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if IsDerivedStoragePath(rel) {
				skippedDerived++
				return nil
			}
			present[rel] = true
			if validateErr := validateStorageFile(rel, path); validateErr != nil {
				logger.SubWarn("Persistence", "Scan", "扫描校验失败(%s): %v", rel, validateErr)
				_ = archiveDB.DeleteResourcesByBlobPath(rel)
				cleaned++
				return nil
			}
			if _, _, syncErr := upsertHandleForPath(rel, path); syncErr != nil {
				logger.SubWarn("Persistence", "Scan", "扫描登记失败(%s): %v", rel, syncErr)
				_ = archiveDB.DeleteResourcesByBlobPath(rel)
				cleaned++
				return nil
			}
			scanned++
			return nil
		})
		if walkErr != nil && !os.IsNotExist(walkErr) {
			logger.SubWarn("Persistence", "Scan", "扫描目录失败(%s): %v", root, walkErr)
		}
	}

	// 清理「记录仍在、实体已丢失」的句柄（仅限三棵内容仓库树）
	rows, err := archiveDB.ListAllResources()
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
		if err := archiveDB.DeleteResource(res.ID); err != nil {
			logger.SubWarn("Persistence", "Scan", "清理失效句柄失败(%s): %v", res.Handle, err)
			continue
		}
		cleaned++
	}
	// 历史遗留的派生缓存索引行一并清理（只删行，不删文件）
	archiveDB.cleanupDerivedRows()

	logger.Info("Persistence", "内容仓库句柄同步完成：登记 %d 个实体，清理 %d 个失效项，跳过 %d 个派生缓存文件",
		scanned, cleaned, skippedDerived)
	return nil
}

// validateStorageFile 扫描期的完整性校验：图片必须能解析出宽高（SVG 除外）
func validateStorageFile(rel, abs string) error {
	ext := strings.ToLower(filepath.Ext(rel))
	if KindOfExt(ext) != KindImage || ext == ".svg" {
		return nil
	}
	if _, _, err := imageDimensions(abs); err != nil {
		return err
	}
	return nil
}
