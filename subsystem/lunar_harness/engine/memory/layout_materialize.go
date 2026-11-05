package memory

// layout_materialize.go 布局准备与物化

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (d *Store) prepareLayout(forceRebuild bool) (ClusterMigrationReport, error) {
	var report ClusterMigrationReport
	if d.baseDir == "" {
		return report, nil
	}
	if err := os.MkdirAll(d.baseDir, 0755); err != nil {
		return report, fmt.Errorf("创建记忆库目录失败: %w", err)
	}

	layout, err := scanMemoryLayout(d.baseDir)
	if err != nil {
		return report, err
	}
	index, modTime, hadIndex := loadClusterIndexFromDisk(d.baseDir)
	if index != nil && !clusterIndexStructurallyValid(index) {
		index = nil
	}

	report.DocCount = len(layout.Docs)
	report.FileCount = layout.FileCount

	if !forceRebuild && index != nil && layoutMatchesIndex(d.baseDir, layout, index) {
		d.setClusterIndex(index, modTime)
		if err := d.loadClusterCollections(index); err != nil {
			return report, err
		}
		report.VectorCount = countDocVectors(layout.Docs)
		report.StoreSize = index.StoreSize
		report.ClusterCount = len(index.Clusters)
		report.AssignedCount = len(index.Assignments)
		report.ReusedIndex = true
		return report, nil
	}

	// 空库也需要建空总库（不建簇、不建 Assignments）
	if len(layout.Docs) == 0 {
		empty := newClusterIndex(currentEmbeddingModel())
		indexDir := clusterDirForID(d.baseDir, StoreCollectionID)
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			return report, fmt.Errorf("创建总库目录失败: %w", err)
		}
		if _, err := writeStoreDir(indexDir, 0, nil); err != nil {
			return report, err
		}
		if err := d.saveClusterIndexFile(empty); err != nil {
			return report, fmt.Errorf("写入空聚类索引失败: %w", err)
		}
		d.setClusterIndex(empty, time.Now())
		if err := d.loadClusterCollections(empty); err != nil {
			return report, err
		}
		report.ReusedIndex = true
		return report, nil
	}

	return d.materializeLayout(layout, index, hadIndex, forceRebuild)
}

// materializeLayout 备份 → 写总库 → 建索引 → 迁出正式簇 → 对账 → 切换 → 删旧目录
func (d *Store) materializeLayout(layout *diskLayout, existing *ClusterIndex, hadIndex, forceRebuild bool) (ClusterMigrationReport, error) {
	report := ClusterMigrationReport{DocCount: len(layout.Docs)}
	docs := sortedDocsFromMap(layout.Docs)

	// 1. 构建索引（决定哪些条目进总库、哪些进正式簇）
	reuse := !forceRebuild && hadIndex && existing != nil &&
		clusterIndexStructurallyValid(existing) &&
		existing.StoreSize+len(existing.Assignments) == len(layout.Docs)
	if reuse {
		for _, node := range existing.Clusters {
			if node.Size < ClusterUpgradeSize {
				reuse = false
				break
			}
		}
	}
	var index *ClusterIndex
	if reuse {
		index = cloneClusterIndexValue(existing)
		report.ReusedIndex = true
	} else {
		index, _ = buildClusterIndexFromDocs(docs, currentEmbeddingModel())
	}
	report.StoreSize = index.StoreSize
	report.ClusterCount = len(index.Clusters)
	report.AssignedCount = len(index.Assignments)
	report.VectorCount = countDocVectors(layout.Docs)
	if err := reconcileClusterIndex(index, report); err != nil {
		return report, fmt.Errorf("聚类迁移对账失败: %w", err)
	}

	// 2. 备份
	backupDir, err := d.backupLayout(layout, report.VectorCount, hadIndex)
	if err != nil {
		return report, err
	}
	report.BackupDir = backupDir

	// 3. 全部条目先写进总库 store-all/
	storeDir := clusterDirForID(d.baseDir, StoreCollectionID)
	storeDocs := make([]Document, 0, len(docs))
	storeDocs = append(storeDocs, docs...)
	if _, err := writeStoreDir(storeDir, index.Dimension, storeDocs); err != nil {
		d.rollbackLayout(backupDir, map[string]bool{storeDir: true})
		return report, fmt.Errorf("写入总库失败: %w", err)
	}
	createdDirs := map[string]bool{storeDir: true}

	// 4. 正式簇：从总库分片里"迁出"到 c-* 簇目录
	moved := 0
	for _, node := range index.Clusters {
		clusterDocs := docsByIDs(docs, node.DocIDs)
		if len(clusterDocs) == 0 {
			continue
		}
		dir := clusterDirForID(d.baseDir, node.ID)
		if _, err := writeClusterDir(dir, index.Dimension, clusterDocs); err != nil {
			d.rollbackLayout(backupDir, createdDirs)
			return report, fmt.Errorf("写入簇目录 %s 失败: %w", node.ID, err)
		}
		createdDirs[dir] = true
		moved += len(clusterDocs)

		// 从总库分片里删除这些条目（先写了簇目录才敢删，保证原子性）
		if err := removeDocsFromDir(storeDir, node.DocIDs); err != nil {
			d.rollbackLayout(backupDir, createdDirs)
			return report, fmt.Errorf("从总库移除簇 %s 成员失败: %w", node.ID, err)
		}
	}
	report.MovedDocs = moved

	// 5. 清理索引中不再引用的 c-* 目录（合并后被废弃的簇）
	for _, dir := range layout.ClusterDirs {
		name := filepath.Base(dir)
		if !isClusterDirName(name) {
			continue
		}
		if isExpectedClusterDir(d.baseDir, index, name) {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			d.rollbackLayout(backupDir, createdDirs)
			return report, fmt.Errorf("删除废弃簇目录 %s 失败: %w", dir, err)
		}
	}

	// 6. 对账
	if err := reconcileClusterLayout(d.baseDir, index); err != nil {
		d.rollbackLayout(backupDir, createdDirs)
		return report, fmt.Errorf("磁盘布局对账失败: %w", err)
	}
	report.FileCount = countAllShardFiles(d.baseDir, index)

	// 7. 写 clusters.json（切换点）
	d.clusterMu.Lock()
	if err := d.saveClusterIndexFile(index); err != nil {
		d.clusterMu.Unlock()
		d.rollbackLayout(backupDir, createdDirs)
		return report, fmt.Errorf("写入聚类索引失败: %w", err)
	}
	d.clusterIndex = index
	d.clusterMu.Unlock()

	// 8. 删旧布局目录（切换成功后才删）
	for _, dir := range layout.LegacyDirs {
		if err := os.RemoveAll(dir); err != nil {
			logger.Warn("FileManager", "删除旧目录 %s 失败（新布局已生效，下次重试）: %v", dir, err)
			continue
		}
		report.LegacyDirs++
	}

	// 9. 加载到内存
	if err := d.loadClusterCollections(index); err != nil {
		return report, err
	}

	logger.Info("FileManager", "布局迁移完成：总库 %d + 正式簇 %d（成员 %d），分片 %d，搬迁 %d，删旧目录 %d，备份 %s",
		report.StoreSize, report.ClusterCount, report.AssignedCount,
		report.FileCount, report.MovedDocs, report.LegacyDirs, report.BackupDir)
	return report, nil
}

// docsByIDs 从切片中取出指定 ID 的条目（保留原切片顺序）
func docsByIDs(all []Document, ids []string) []Document {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	out := make([]Document, 0, len(ids))
	for _, doc := range all {
		if wanted[doc.ID] {
			out = append(out, doc)
		}
	}
	return out
}

// countDocVectors 统计带向量条目数
func countDocVectors(docs map[string]Document) int {
	count := 0
	for _, doc := range docs {
		if len(doc.Embedding) > 0 {
			count++
		}
	}
	return count
}

// countAllShardFiles 统计总库 + 全部正式簇的分片文件总数
func countAllShardFiles(baseDir string, index *ClusterIndex) int {
	count := countShardFiles(clusterDirForID(baseDir, StoreCollectionID))
	for _, node := range index.Clusters {
		count += countShardFiles(clusterDirForID(baseDir, node.ID))
	}
	return count
}

// =============================================================================
// 磁盘写入 / 对账
// =============================================================================

// writeStoreDir 把总库的全部条目写成统一分片（100 条/块）
