package memory

// layout_backup.go 备份与回滚

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func reconcileClusterLayout(baseDir string, index *ClusterIndex) error {
	if index == nil {
		return fmt.Errorf("聚类索引为空")
	}

	// 1. 总库目录存在
	storeDir := clusterDirForID(baseDir, StoreCollectionID)
	if _, err := os.Stat(storeDir); err != nil {
		return fmt.Errorf("总库目录 store-all 缺失: %w", err)
	}

	// 2. 正式簇目录条目数与索引一致
	assigned := 0
	for _, node := range index.Clusters {
		if node.Size == 0 {
			return fmt.Errorf("空簇 %s 不应保留目录", node.ID)
		}
		dir := clusterDirForID(baseDir, node.ID)
		if _, err := os.Stat(dir); err != nil {
			return fmt.Errorf("簇 %s 目录缺失: %w", node.ID, err)
		}
		docs, docChunks, embChunks, err := readDocumentsFromDir(dir)
		if err != nil {
			return err
		}
		if len(docs) != node.Size {
			return fmt.Errorf("簇 %s 目录条目数 %d ≠ 索引规模 %d", node.ID, len(docs), node.Size)
		}
		expectChunks := (node.Size + DocumentsChunkSize - 1) / DocumentsChunkSize
		if docChunks != expectChunks || embChunks != expectChunks {
			return fmt.Errorf("簇 %s 分片数 documents=%d embeddings=%d ≠ 期望 %d",
				node.ID, docChunks, embChunks, expectChunks)
		}
		for _, doc := range docs {
			if index.Assignments[doc.ID] != node.ID {
				return fmt.Errorf("条目 %s 实际位于簇 %s，索引归属 %q", doc.ID, node.ID, index.Assignments[doc.ID])
			}
		}
		assigned += len(docs)
	}

	// 3. 总库条目 + 正式簇成员 = StoreSize + Assignments
	storeDocs, _, _, err := readDocumentsFromDir(storeDir)
	if err != nil {
		return err
	}
	if len(storeDocs) != index.StoreSize {
		return fmt.Errorf("总库条目数 %d ≠ 索引 StoreSize %d", len(storeDocs), index.StoreSize)
	}
	if assigned != len(index.Assignments) {
		return fmt.Errorf("正式簇成员合计 %d ≠ Assignments %d", assigned, len(index.Assignments))
	}

	// 4. 总库条目不在 Assignments 里
	for _, doc := range storeDocs {
		if _, inCluster := index.Assignments[doc.ID]; inCluster {
			return fmt.Errorf("总库条目 %s 同时出现在 Assignments 里（矛盾）", doc.ID)
		}
	}

	// 5. 没有索引未引用的非空 c-* 目录
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !isClusterDirName(entry.Name()) {
			continue
		}
		if isExpectedClusterDir(baseDir, index, entry.Name()) {
			continue
		}
		dir := filepath.Join(baseDir, entry.Name())
		docs, _, _, _ := readDocumentsFromDir(dir)
		if len(docs) > 0 {
			return fmt.Errorf("存在索引未引用的非空簇目录 %s", entry.Name())
		}
	}

	return nil
}

// =============================================================================
// 备份 / 回滚
// =============================================================================

// backupLayout 备份旧索引、目录分片与清单
func (d *Store) backupLayout(layout *diskLayout, vectorCount int, hadIndex bool) (string, error) {
	stamp := time.Now().UnixNano()
	backupDir := filepath.Join(d.baseDir, ClusterMigrateBackupDir, strconv.FormatInt(stamp, 10))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("创建迁移备份目录失败: %w", err)
	}

	// 备份旧 clusters.json（如果存在）
	if data, err := os.ReadFile(clusterIndexPath(d.baseDir)); err == nil {
		hadIndex = true
		if err := os.WriteFile(filepath.Join(backupDir, ClusterIndexFile), data, 0644); err != nil {
			return "", fmt.Errorf("备份旧聚类索引失败: %w", err)
		}
	}

	dirs := make(map[string]int, len(layout.LegacyDirs)+len(layout.ClusterDirs))
	for _, dir := range layout.LegacyDirs {
		dirs[filepath.Base(dir)] = countShardFiles(dir)
	}
	for _, dir := range layout.ClusterDirs {
		dirs[filepath.Base(dir)] = countShardFiles(dir)
	}

	manifest := ClusterBackupManifest{
		Timestamp:       time.Now().Unix(),
		HadClusterIndex: hadIndex,
		DocCount:        len(layout.Docs),
		VectorCount:     vectorCount,
		Dirs:            dirs,
		LegacyDirs:      layout.LegacyDirs,
	}
	if err := atomicWriteJSON(filepath.Join(backupDir, ClusterLayoutManifest), manifest); err != nil {
		return "", fmt.Errorf("写入备份清单失败: %w", err)
	}

	// 备份既有簇目录分片（总库目录也可能被重写）
	allBackup := append([]string(nil), layout.ClusterDirs...)
	if layout.StoreDir != "" {
		allBackup = append(allBackup, layout.StoreDir)
	}
	for _, dir := range allBackup {
		name := filepath.Base(dir)
		dst := filepath.Join(backupDir, "dirs", name)
		if err := copyDirShards(dir, dst); err != nil {
			return "", fmt.Errorf("备份目录 %s 失败: %w", name, err)
		}
	}

	return backupDir, nil
}

// rollbackLayout 迁移失败时回滚：删除本次新建目录、恢复旧簇目录与索引
func (d *Store) rollbackLayout(backupDir string, createdDirs map[string]bool) {
	for dir := range createdDirs {
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			logger.Error("FileManager", "迁移回滚：删除新建目录 %s 失败: %v", dir, err)
		}
	}
	if backupDir == "" {
		return
	}

	// 恢复旧目录分片
	backupDirs, _ := os.ReadDir(filepath.Join(backupDir, "dirs"))
	for _, entry := range backupDirs {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		src := filepath.Join(backupDir, "dirs", name)
		dst := filepath.Join(d.baseDir, name)
		if err := os.RemoveAll(dst); err != nil && !os.IsNotExist(err) {
			logger.Error("FileManager", "迁移回滚：清理 %s 失败: %v", name, err)
			continue
		}
		if err := copyDirShards(src, dst); err != nil {
			logger.Error("FileManager", "迁移回滚：恢复 %s 失败: %v", name, err)
		}
	}

	// 恢复旧索引
	target := clusterIndexPath(d.baseDir)
	backup := filepath.Join(backupDir, ClusterIndexFile)
	if data, err := os.ReadFile(backup); err == nil {
		if writeErr := os.WriteFile(target, data, 0644); writeErr != nil {
			logger.Error("FileManager", "迁移回滚：恢复旧索引失败: %v", writeErr)
		}
	} else if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		logger.Error("FileManager", "迁移回滚：删除新索引失败: %v", err)
	}

	logger.Warn("FileManager", "聚类迁移已回滚（旧布局目录未删除），备份位于 %s", backupDir)
}

// copyDirShards 复制目录内的分片与元数据文件
func copyDirShards(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}
	patterns := []string{"documents_*.json", "embeddings_*.json", "metadata.json"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(filepath.Join(srcDir, pattern))
		for _, src := range files {
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dstDir, filepath.Base(src)), data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

// =============================================================================
// 运行期搬迁（用于降级）
// =============================================================================

// collectionByName 按 ID 取内存中的存储单元
