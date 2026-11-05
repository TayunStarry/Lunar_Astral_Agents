package memory

// cluster_index.go 向量空间聚类索引（按职责拆分）

import (
	"sort"
	"time"
)

func buildClusterIndexFromDocs(docs []Document, model string) (*ClusterIndex, int) {
	sorted := sortedDocsByTime(docs)

	index := newClusterIndex(model)
	if len(sorted) == 0 {
		return index, 0
	}

	// 第一轮：带向量条目先做代表式聚类
	type rawGroup struct {
		rep      Document  // 代表条目（首条，按时间）
		members  []string  // 组内成员 ID
		centroid []float32 // 组质心
	}
	groups := make([]rawGroup, 0)
	vectors := make(map[string][]float32, len(sorted))

	for _, doc := range sorted {
		if doc.ID == "" {
			continue
		}
		if len(doc.Embedding) == 0 {
			continue // 无向量 → 留在总库
		}
		if index.Dimension == 0 {
			index.Dimension = len(doc.Embedding)
		}
		if len(doc.Embedding) != index.Dimension {
			continue // 维度不符 → 留在总库
		}
		vectors[doc.ID] = doc.Embedding

		bestIdx := -1
		bestSim := float32(0)
		for i := range groups {
			sim := cosineSimilarity(doc.Embedding, groups[i].rep.Embedding)
			if sim >= ClusterAdmitThreshold && sim > bestSim {
				bestSim = sim
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			groups[bestIdx].members = append(groups[bestIdx].members, doc.ID)
			groups[bestIdx].centroid = mixAndNormalize(groups[bestIdx].centroid, len(groups[bestIdx].members)-1, doc.Embedding)
			continue
		}
		// 新组
		groups = append(groups, rawGroup{
			rep:      doc,
			members:  []string{doc.ID},
			centroid: normalizeVector(doc.Embedding),
		})
	}

	// 第二轮：过滤升级门槛
	for _, g := range groups {
		if len(g.members) >= ClusterUpgradeSize {
			// 正式簇
			node := ClusterNode{
				ID:        "cluster-" + g.rep.ID,
				Centroid:  g.centroid,
				DocIDs:    g.members,
				Size:      len(g.members),
				UpdatedAt: time.Now().Unix(),
			}
			index.Clusters = append(index.Clusters, node)
			for _, id := range g.members {
				index.Assignments[id] = node.ID
			}
		}
		// < ClusterUpgradeSize 的组：成员全留总库，不写 Assignments
	}

	// 第三轮：正式簇之间合并邻近 / 分裂超大
	index.mergeNearby(vectors)
	index.splitOversized(vectors)

	// 最终对账 StoreSize / StoreVectors
	totalVectors := 0
	totalDocs := len(sorted)
	assignedVectors := 0
	for _, node := range index.Clusters {
		for _, id := range node.DocIDs {
			if _, ok := vectors[id]; ok {
				assignedVectors++
			}
		}
	}
	for _, doc := range sorted {
		if doc.ID == "" {
			continue
		}
		if len(doc.Embedding) > 0 && (index.Dimension == 0 || len(doc.Embedding) == index.Dimension) {
			totalVectors++
		}
	}
	index.StoreSize = totalDocs - len(index.Assignments)
	index.StoreVectors = totalVectors - assignedVectors
	index.UpdatedAt = time.Now().Unix()

	return index, 0
}

// mixAndNormalize 把新向量并入已有质心（权重为旧成员数 + 1）并返回单位向量。
func mixAndNormalize(centroid []float32, oldCount int, vec []float32) []float32 {
	return normalizeVector(mixCentroid(centroid, oldCount, vec))
}

// mergeNearby 合并质心余弦达阈值的正式簇（重建阶段执行，保持确定性）。
func (index *ClusterIndex) mergeNearby(vectors map[string][]float32) {
	for {
		merged := false
		for i := 0; i < len(index.Clusters); i++ {
			for j := i + 1; j < len(index.Clusters); j++ {
				if index.Clusters[i].Size+index.Clusters[j].Size > ClusterMaxMembers {
					continue
				}
				if cosineSimilarity(index.Clusters[i].Centroid, index.Clusters[j].Centroid) < ClusterMergeThreshold {
					continue
				}
				// 成员更多的簇保留 ID；规模相同按 ID 升序保留前者，保证确定性
				from, to := j, i
				if index.Clusters[j].Size > index.Clusters[i].Size ||
					(index.Clusters[j].Size == index.Clusters[i].Size && index.Clusters[j].ID < index.Clusters[i].ID) {
					from, to = i, j
				}
				mergeClusterNodes(&index.Clusters[to], &index.Clusters[from], index.Assignments)
				index.Clusters = append(index.Clusters[:from], index.Clusters[from+1:]...)
				merged = true
				break
			}
			if merged {
				break
			}
		}
		if !merged {
			return
		}
	}
}

// mergeClusterNodes 把 from 的成员并入 to，并更新归属映射。
func mergeClusterNodes(to, from *ClusterNode, assignments map[string]string) {
	for _, id := range from.DocIDs {
		to.DocIDs = append(to.DocIDs, id)
		assignments[id] = to.ID
	}
	to.Size = len(to.DocIDs)
	to.Centroid = normalizeVector(mixCentroid(to.Centroid, to.Size-len(from.DocIDs), from.Centroid))
}

// splitOversized 分裂超过成员上限的正式簇。
func (index *ClusterIndex) splitOversized(vectors map[string][]float32) int {
	splits := 0
	attempted := make(map[string]bool, len(index.Clusters))
	for {
		target := -1
		for i := range index.Clusters {
			node := &index.Clusters[i]
			if attempted[node.ID] {
				continue
			}
			if node.Size > ClusterMaxMembers {
				target = i
				break
			}
		}
		if target < 0 {
			return splits
		}
		node := index.Clusters[target]
		attempted[node.ID] = true
		groups := regroupCluster(node, vectors)
		if len(groups) < 2 {
			continue // 簇内向量无法再分
		}
		// 最大子簇沿用原 ID
		sort.SliceStable(groups, func(i, j int) bool { return len(groups[i].DocIDs) > len(groups[j].DocIDs) })
		keep := groups[0]
		keep.ID = node.ID
		index.Clusters[target] = keep
		for _, group := range groups[1:] {
			index.Clusters = append(index.Clusters, group)
			attempted[group.ID] = true
			splits++
		}
		for _, group := range groups {
			for _, id := range group.DocIDs {
				index.Assignments[id] = group.ID
			}
		}
	}
}

// regroupCluster 按簇内向量重新划分（代表式聚类）。
func regroupCluster(node ClusterNode, vectors map[string][]float32) []ClusterNode {
	ids := append([]string(nil), node.DocIDs...)
	sort.Strings(ids)
	groups := make([]ClusterNode, 0, 2)
	for _, id := range ids {
		vec, ok := vectors[id]
		if !ok || len(vec) == 0 {
			return nil // 放弃分裂，避免残缺
		}
		best := -1
		bestSim := float32(0)
		for i := range groups {
			sim := cosineSimilarity(vec, groups[i].Centroid)
			if sim >= ClusterAdmitThreshold && sim > bestSim {
				bestSim = sim
				best = i
			}
		}
		if best >= 0 {
			appendClusterMember(&groups[best], id, vec, 0)
			continue
		}
		groups = append(groups, ClusterNode{
			ID:       "cluster-" + id,
			Centroid: normalizeVector(vec),
			DocIDs:   []string{id},
			Size:     1,
		})
	}
	return groups
}

// appendClusterMember 追加成员并更新质心
func appendClusterMember(node *ClusterNode, docID string, vec []float32, stamp int64) {
	node.DocIDs = append(node.DocIDs, docID)
	node.Size = len(node.DocIDs)
	node.Centroid = normalizeVector(mixCentroid(node.Centroid, node.Size-1, vec))
	node.UpdatedAt = stamp
}

// averageClusterSimilarity 簇内成员与质心的平均余弦（保留以备将来紧凑度判定）
func averageClusterSimilarity(node *ClusterNode, vectors map[string][]float32) float32 {
	if node.Size == 0 {
		return 0
	}
	var sum float32
	count := 0
	for _, id := range node.DocIDs {
		vec, ok := vectors[id]
		if !ok || len(vec) != len(node.Centroid) {
			continue
		}
		sum += cosineSimilarity(vec, node.Centroid)
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float32(count)
}

// memberIDs 返回正式簇全部条目 ID（用于批量取向量）。
