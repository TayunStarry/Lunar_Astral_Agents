package memory

// cluster_index.go 向量空间聚类索引（按职责拆分）

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"fmt"
	"time"
)

func (d *Store) assignToStore(docID string, vec []float32) error {
	if d.baseDir == "" || docID == "" {
		return nil // 纯内存实例不维护持久化索引
	}

	d.clusterMu.Lock()
	defer d.clusterMu.Unlock()

	index := d.clusterIndex
	if index == nil {
		index = newClusterIndex(currentEmbeddingModel())
		d.clusterIndex = index
	}
	if index.Dimension == 0 && len(vec) > 0 {
		index.Dimension = len(vec)
	}
	if len(vec) > 0 && index.Dimension > 0 && index.Dimension != len(vec) {
		return fmt.Errorf("条目 %s 向量维度 %d 与索引维度 %d 不一致", docID, len(vec), index.Dimension)
	}
	index.StoreSize++
	if len(vec) > 0 && (index.Dimension == 0 || len(vec) == index.Dimension) {
		index.StoreVectors++
	}
	index.UpdatedAt = time.Now().Unix()
	return d.saveClusterIndexFile(index)
}

// detachCluster 删除条目后同步维护索引：
//   - 如果条目在总库：StoreSize--
//   - 如果条目在正式簇：从簇内移除，簇删到 < ClusterDowngradeSize(8) 时解散回总库
func (d *Store) detachCluster(docID string) {
	if docID == "" {
		return
	}
	d.clusterMu.Lock()
	index := d.clusterIndex
	if index == nil {
		d.clusterMu.Unlock()
		return
	}

	// 1. 先尝试从正式簇移除
	clusterID, inCluster := index.Assignments[docID]
	removedFromCluster := false
	if inCluster {
		delete(index.Assignments, docID)
		for i := range index.Clusters {
			node := &index.Clusters[i]
			if node.ID != clusterID {
				continue
			}
			remaining := node.DocIDs[:0]
			for _, id := range node.DocIDs {
				if id != docID {
					remaining = append(remaining, id)
				}
			}
			node.DocIDs = remaining
			node.Size = len(remaining)
			if node.Size == 0 {
				index.Clusters = append(index.Clusters[:i], index.Clusters[i+1:]...)
			}
			break
		}
		removedFromCluster = true
	}

	// 2. 不在正式簇 = 在总库
	if !removedFromCluster {
		if index.StoreSize > 0 {
			index.StoreSize--
		}
		// StoreVectors 无法精确跟踪（总库可能同时存在多条带向量条目），保守不--
	}

	index.UpdatedAt = time.Now().Unix()
	if err := d.saveClusterIndexFile(index); err != nil {
		logger.Warn("FileManager", "聚类索引：移除条目 %s 后保存失败: %v", docID, err)
	}
	d.clusterMu.Unlock()

	// 3. 正式簇删到 < 降级门槛 → 解散回总库
	if removedFromCluster {
		d.downgradeClusterIfBelowThreshold(clusterID)
	}
}

// downgradeClusterIfBelowThreshold 运行期降级：簇内删除到 < 8 条 → 解散回总库。
func (d *Store) downgradeClusterIfBelowThreshold(clusterID string) {
	if clusterID == "" {
		return
	}
	d.clusterMu.Lock()
	index := d.clusterIndex
	if index == nil {
		d.clusterMu.Unlock()
		return
	}
	for i := range index.Clusters {
		node := &index.Clusters[i]
		if node.ID != clusterID {
			continue
		}
		if node.Size >= ClusterDowngradeSize {
			d.clusterMu.Unlock()
			return
		}

		// 把剩余成员的归属从 Assignments 移除 → 它们变成总库条目
		for _, id := range node.DocIDs {
			delete(index.Assignments, id)
			index.StoreSize++
		}
		index.Clusters = append(index.Clusters[:i], index.Clusters[i+1:]...)
		index.UpdatedAt = time.Now().Unix()
		if err := d.saveClusterIndexFile(index); err != nil {
			logger.Warn("FileManager", "聚类索引：簇 %s 降级保存失败: %v", clusterID, err)
		}
		d.clusterMu.Unlock()

		// 先写新（总库）、再删旧（簇目录）
		if err := d.moveClusterDocsToStore(clusterID); err != nil {
			logger.Warn("FileManager", "降级：簇 %s 搬回总库失败: %v", clusterID, err)
		}
		d.removeClusterStorage(clusterID)
		logger.Info("FileManager", "簇 %s 降级解散回总库（剩余 %d 条）", clusterID, node.Size)
		return
	}
	d.clusterMu.Unlock()
}

// moveClusterDocsToStore 把簇目录内全部条目写进总库（用于降级）。
func (d *Store) moveClusterDocsToStore(clusterID string) error {
	cluster, err := d.clusterForCollection(clusterID)
	if err != nil {
		return err
	}
	if len(cluster.Documents) == 0 {
		return nil
	}
	store, err := d.storeCollection()
	if err != nil {
		return err
	}
	cluster.mu.RLock()
	batch := append([]Document(nil), cluster.Documents...)
	cluster.mu.RUnlock()
	store.mu.Lock()
	store.Documents = append(store.Documents, batch...)
	store.rebuildPositionsLocked()
	store.mu.Unlock()
	if err := store.saveDocumentsToFile(); err != nil {
		return fmt.Errorf("写入总库失败: %w", err)
	}
	store.updateLastModTime()
	// 条目正文与 ID 未变，倒排记录无需重建，只需刷新集合归属
	d.keywordIndexRelocate(documentIDs(batch), store.Name)
	return nil
}

// dropClusterFromIndex 删除整个正式簇：移除全部归属与节点并落盘。
func (d *Store) dropClusterFromIndex(clusterID string) {
	if clusterID == "" {
		return
	}
	d.clusterMu.Lock()
	index := d.clusterIndex
	if index == nil {
		d.clusterMu.Unlock()
		return
	}
	for i := range index.Clusters {
		if index.Clusters[i].ID != clusterID {
			continue
		}
		for _, id := range index.Clusters[i].DocIDs {
			delete(index.Assignments, id)
			index.StoreSize++ // 成员解散回总库
		}
		index.Clusters = append(index.Clusters[:i], index.Clusters[i+1:]...)
		break
	}
	index.UpdatedAt = time.Now().Unix()
	if d.baseDir != "" {
		if err := d.saveClusterIndexFile(index); err != nil {
			logger.Warn("FileManager", "聚类索引：删除簇 %s 后保存失败: %v", clusterID, err)
		}
	}
	d.clusterMu.Unlock()
}

// =============================================================================
// 重建 / 迁移
// =========================================================================

// MigrateClusters 强制按当前条目重新聚类（扫总库、标升级簇、迁出簇目录），
// 迁移完成后扫描全部正式簇做一次降级清理（簇内 < ClusterDowngradeSize 解散回总库）。
func (d *Store) MigrateClusters() (ClusterMigrationReport, error) {
	if d.baseDir == "" {
		return ClusterMigrationReport{}, fmt.Errorf("记忆库 baseDir 为空，无法迁移聚类索引")
	}
	report, err := d.prepareLayout(true)
	if err != nil {
		return report, err
	}

	// 扫尾：重建只会产生 ≥ ClusterUpgradeSize 的簇，但复用/异常索引可能残留 undersized 簇，
	// 这里统一按降级规则收敛，保证"簇内不足下限即回总库"在迁移路径同样成立。
	d.sweepUndersizedClusters()

	// 用扫尾后的索引刷新对账数字，确保 StoreSize + AssignedCount == DocCount 仍然成立
	if index := d.cloneClusterIndex(); index != nil {
		report.StoreSize = index.StoreSize
		report.ClusterCount = len(index.Clusters)
		report.AssignedCount = len(index.Assignments)
	}
	// 迁移只改变条目所在集合，倒排记录不变；刷新集合归属即可
	d.refreshKeywordIndexCollections()
	return report, nil
}

// sweepUndersizedClusters 扫描全部正式簇，簇内 < ClusterDowngradeSize 的解散回总库。
//
// 返回被解散的簇数量。迁移与启动后的清理都复用这一段逻辑，避免只在 Delete 路径降级。
func (d *Store) sweepUndersizedClusters() int {
	index := d.cloneClusterIndex()
	if index == nil {
		return 0
	}
	undersized := make([]string, 0, len(index.Clusters))
	for _, node := range index.Clusters {
		if node.Size < ClusterDowngradeSize {
			undersized = append(undersized, node.ID)
		}
	}
	for _, clusterID := range undersized {
		d.downgradeClusterIfBelowThreshold(clusterID)
	}
	if len(undersized) > 0 {
		logger.Info("FileManager", "聚类扫尾：%d 个不足 %d 条的簇已解散回总库",
			len(undersized), ClusterDowngradeSize)
	}
	return len(undersized)
}

// buildClusterIndexFromDocs 构建"总库为主 + 正式簇"的索引。
//
// 步骤：
//  1. 按（入库时间, ID）排序；
//  2. 无向量条目 + 向量维度不符 → 留在总库；
//  3. 带向量且维度一致 → 用代表式聚类（阈值 ClusterAdmitThreshold=0.90）分组；
//  4. 组内 ≥ ClusterUpgradeSize(16) → 正式簇（写 Assignments）；<16 → 留在总库；
