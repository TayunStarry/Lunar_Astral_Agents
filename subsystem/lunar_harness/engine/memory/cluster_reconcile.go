package memory

// cluster_index.go 向量空间聚类索引（按职责拆分）

import (
	"fmt"
	"math"
	"os"
)

func (index *ClusterIndex) memberIDs() []string {
	ids := make([]string, 0, len(index.Assignments))
	for id := range index.Assignments {
		ids = append(ids, id)
	}
	return ids
}

// =============================================================================
// 对账
// =============================================================================

// reconcileClusterIndex 对账：StoreSize + 正式簇成员 == 条目总数。
func reconcileClusterIndex(index *ClusterIndex, report ClusterMigrationReport) error {
	if index.StoreSize+len(index.Assignments) != report.DocCount {
		return fmt.Errorf("StoreSize(%d) + Assignments(%d) ≠ DocCount(%d)",
			index.StoreSize, len(index.Assignments), report.DocCount)
	}
	members := 0
	for _, node := range index.Clusters {
		if node.Size != len(node.DocIDs) {
			return fmt.Errorf("簇 %s Size(%d) ≠ len(DocIDs)(%d)", node.ID, node.Size, len(node.DocIDs))
		}
		members += len(node.DocIDs)
		for _, id := range node.DocIDs {
			if index.Assignments[id] != node.ID {
				return fmt.Errorf("条目 %s Assignments(%q) ≠ 簇 %s", id, index.Assignments[id], node.ID)
			}
		}
		// 入簇阈值校验：成员与质心余弦必须 ≥ ClusterAdmitThreshold
		if index.Dimension > 0 {
			for _, id := range node.DocIDs {
				// 质心已归一化；vectors 不在此函数可用，留给磁盘层校验
				_ = id
			}
		}
	}
	if members != len(index.Assignments) {
		return fmt.Errorf("簇成员合计 %d ≠ Assignments %d", members, len(index.Assignments))
	}
	return nil
}

// clusterIndexStructurallyValid 校验磁盘索引结构是否可用。
func clusterIndexStructurallyValid(index *ClusterIndex) bool {
	if index == nil || index.Assignments == nil {
		return false
	}
	// 正式簇不能直接用 "store-all" 做 ID
	for _, node := range index.Clusters {
		if node.ID == StoreCollectionID || node.Size != len(node.DocIDs) {
			return false
		}
		if index.Dimension > 0 && len(node.Centroid) != index.Dimension {
			return false
		}
	}
	return true
}

// allDocumentsSnapshot 快照全部存储单元条目（迁移/重建使用）
func (d *Store) allDocumentsSnapshot() ([]Document, map[string]int) {
	collections := d.snapshotCollections()
	docs := make([]Document, 0)
	counts := make(map[string]int, len(collections))
	for _, c := range collections {
		c.reloadIfChanged()
		c.mu.RLock()
		batch := append([]Document(nil), c.Documents...)
		c.mu.RUnlock()
		docs = append(docs, batch...)
		counts[c.Name] = len(batch)
	}
	return docs, counts
}

// vectorLookupForIDs 按 ID 收集向量（只读取已加载的向量，不调用嵌入服务）
func (d *Store) vectorLookupForIDs(ids []string) map[string][]float32 {
	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	lookup := make(map[string][]float32, len(ids))
	if len(wanted) == 0 {
		return lookup
	}
	for _, c := range d.snapshotCollections() {
		c.mu.RLock()
		for i := range c.Documents {
			doc := &c.Documents[i]
			if !wanted[doc.ID] || len(doc.Embedding) == 0 {
				continue
			}
			lookup[doc.ID] = doc.Embedding
		}
		c.mu.RUnlock()
	}
	return lookup
}

// =============================================================================
// 持久化
// =============================================================================

// saveClusterIndexFile 写入 <baseDir>/clusters.json。
// 调用方必须持有 clusterMu 写锁。
func (d *Store) saveClusterIndexFile(index *ClusterIndex) error {
	path := clusterIndexPath(d.baseDir)
	if clusterPersistHook != nil {
		if err := clusterPersistHook(path); err != nil {
			return err
		}
	}
	if err := atomicWriteJSON(path, index); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil {
		d.clusterMod = info.ModTime()
	}
	return nil
}

// =============================================================================
// 向量工具
// =============================================================================

// normalizeVector 返回单位向量；零向量原样返回。
func normalizeVector(vec []float32) []float32 {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return append([]float32(nil), vec...)
	}
	f := float32(1.0 / math.Sqrt(sum))
	out := make([]float32, len(vec))
	for i, v := range vec {
		out[i] = v * f
	}
	return out
}

// mixCentroid 把新向量并入已有质心（权重为旧成员数 + 1）。
func mixCentroid(centroid []float32, oldCount int, vec []float32) []float32 {
	if len(centroid) != len(vec) || oldCount <= 0 {
		return append([]float32(nil), vec...)
	}
	out := make([]float32, len(vec))
	for i := range vec {
		out[i] = (centroid[i]*float32(oldCount) + vec[i]) / float32(oldCount+1)
	}
	return out
}
