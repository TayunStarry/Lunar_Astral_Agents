package memory

// cluster_index.go 向量空间聚类索引（按职责拆分）

import (
	"path/filepath"
	"time"
)

var clusterPersistHook func(path string) error

// clusterIndexPath 聚类索引文件路径
func clusterIndexPath(baseDir string) string {
	return filepath.Join(baseDir, ClusterIndexFile)
}

// newClusterIndex 新建空索引
func newClusterIndex(model string) *ClusterIndex {
	return &ClusterIndex{
		Version:     ClusterIndexVersion,
		Threshold:   ClusterAdmitThreshold,
		Model:       model,
		Assignments: make(map[string]string),
	}
}

// =============================================================================
// 读取 / 初始化
// =============================================================================

// ensureClusterIndex 确保内存中已有聚类索引（幂等，可重复调用）。
//
// 索引与磁盘布局由 prepareLayout 统一准备：布局一致时直接加载，不一致时
// 走备份 → 先写总库 → 标升级簇 → 迁出簇目录 → 对账 → 切换 → 删旧目录。
func (d *Store) ensureClusterIndex() error {
	if d.baseDir == "" {
		return nil
	}
	d.clusterMu.RLock()
	ready := d.clusterIndex != nil
	d.clusterMu.RUnlock()
	if ready {
		return nil
	}
	_, err := d.prepareLayout(false)
	return err
}

// setClusterIndex 更新内存索引与加载时间
func (d *Store) setClusterIndex(index *ClusterIndex, modTime time.Time) {
	d.clusterMu.Lock()
	d.clusterIndex = index
	d.clusterMod = modTime
	d.clusterMu.Unlock()
}

// cloneClusterIndex 返回索引深拷贝，避免调用方在锁外读取可变结构。
func (d *Store) cloneClusterIndex() *ClusterIndex {
	d.clusterMu.RLock()
	defer d.clusterMu.RUnlock()
	return cloneClusterIndexValue(d.clusterIndex)
}

func cloneClusterIndexValue(index *ClusterIndex) *ClusterIndex {
	if index == nil {
		return nil
	}
	out := &ClusterIndex{
		Version:      index.Version,
		Threshold:    index.Threshold,
		Model:        index.Model,
		Dimension:    index.Dimension,
		StoreSize:    index.StoreSize,
		StoreVectors: index.StoreVectors,
		UpdatedAt:    index.UpdatedAt,
		Clusters:     make([]ClusterNode, 0, len(index.Clusters)),
		Assignments:  make(map[string]string, len(index.Assignments)),
	}
	for _, node := range index.Clusters {
		out.Clusters = append(out.Clusters, ClusterNode{
			ID:        node.ID,
			Centroid:  append([]float32(nil), node.Centroid...),
			DocIDs:    append([]string(nil), node.DocIDs...),
			Size:      node.Size,
			UpdatedAt: node.UpdatedAt,
		})
	}
	for id, clusterID := range index.Assignments {
		out.Assignments[id] = clusterID
	}
	return out
}

// =============================================================================
// 写入进总库（新语义：不自动归簇）
// =============================================================================

// assignToStore 把条目登记到总库（store-all）的索引中，不做 Document 追加。
// 调用方负责把完整的 Document 写入总库集合。此函数只维护 StoreSize/StoreVectors
// 计数、维度锁定与 clusters.json 持久化。
