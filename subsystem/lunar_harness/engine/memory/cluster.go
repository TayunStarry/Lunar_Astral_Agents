package memory

// cluster.go 相似候选的只读聚类与整理结果向量校验。
//
// 本文件不写任何记忆：聚类结果交给上层（chain/life 的整理事件或 Fuse 管理端点），
// 写回统一走 ReplaceTexts 的「先写新、后删旧、校验、回滚」事务。

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ClusterTextCandidates 按内容向量对候选文本条目做代表式聚类，只返回 ≥2 条的簇。
// collectionName 为空表示扫描全部集合；ids 非空时只把这些条目纳入候选。
// 返回的簇按入库时间升序排列，便于上层按“最久优先”处理。
func (d *Store) ClusterTextCandidates(ctx context.Context, collectionName string, ids []string, threshold float32) ([]TextCluster, int, error) {
	clusters, candidates, _, err := d.clusterTextCandidates(ctx, collectionName, ids, threshold, 0)
	return clusters, candidates, err
}

// clusterTextCandidates 聚类实现；maxCandidates>0 时超过上限直接报错（避免 O(n²) 失控）。
func (d *Store) clusterTextCandidates(ctx context.Context, collectionName string, ids []string, threshold float32, maxCandidates int) ([]TextCluster, int, int, error) {
	docs, skipped, err := d.collectTextCandidates(collectionName, ids)
	if err != nil {
		return nil, 0, skipped, err
	}
	if maxCandidates > 0 && len(docs) > maxCandidates {
		return nil, len(docs), skipped, fmt.Errorf("本次候选 %d 条，超过单次聚类上限 %d 条；请缩小候选范围", len(docs), maxCandidates)
	}
	if len(docs) < 2 {
		return nil, len(docs), skipped, nil
	}

	model := currentEmbeddingModel()
	valid := make([]Document, 0, len(docs))
	for _, doc := range docs {
		if len(doc.Embedding) == 0 {
			vec, embedErr := d.embedText(ctx, model, doc.Content)
			if embedErr != nil {
				logger.Warn("FileManager", "聚类跳过 %s：嵌入失败 %v", doc.ID, embedErr)
				skipped++
				continue
			}
			doc.Embedding = vec
		}
		valid = append(valid, doc)
	}
	if len(valid) < 2 {
		return nil, len(docs), skipped, nil
	}

	// 以首条维度为准，避免不同模型产生的向量混算
	dimension := len(valid[0].Embedding)
	aligned := valid[:0]
	for _, doc := range valid {
		if len(doc.Embedding) != dimension {
			skipped++
			continue
		}
		aligned = append(aligned, doc)
	}
	valid = aligned
	if len(valid) < 2 {
		return nil, len(docs), skipped, nil
	}

	threshold = normalizeClusterThreshold(threshold)
	raw := clusterTextDocs(valid, threshold, ClusterMaxItems, ClusterMaxRunes)
	clusters := make([]TextCluster, 0, len(raw))
	for _, cluster := range raw {
		if len(cluster.indices) < 2 {
			continue
		}
		groupDocs := make([]Document, 0, len(cluster.indices))
		for _, index := range cluster.indices {
			groupDocs = append(groupDocs, valid[index])
		}
		item := TextCluster{
			IDs:             make([]string, 0, len(groupDocs)),
			Contents:        make([]string, 0, len(groupDocs)),
			Roles:           make([]string, 0, len(groupDocs)),
			Role:            majorityRole(groupDocs),
			MinSimilarity:   cluster.minSimilarity,
			OldestTimestamp: groupDocs[0].Timestamp,
			NewestTimestamp: groupDocs[len(groupDocs)-1].Timestamp,
		}
		for _, doc := range groupDocs {
			item.IDs = append(item.IDs, doc.ID)
			item.Contents = append(item.Contents, doc.Content)
			item.Roles = append(item.Roles, doc.Role)
		}
		clusters = append(clusters, item)
	}
	return clusters, len(docs), skipped, nil
}

// VerifyReplacementSimilarity 校验整理结果与来源记忆的双向语义一致性：
//   - 每条新文本都要与至少一条来源记忆足够相似（防止模型改写跑偏/凭空发挥）；
//   - 每条来源记忆都要被至少一条新文本覆盖（防止丢事实）。
//
// 任一方向不满足即返回错误，上层据此保留原条目不动。
func (d *Store) VerifyReplacementSimilarity(ctx context.Context, oldIDs []string, newTexts []string, threshold float32) error {
	texts := normalizeTextList(newTexts)
	if len(texts) == 0 {
		return fmt.Errorf("整理结果为空")
	}
	docs, _, err := d.collectTextCandidates("", oldIDs)
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		return fmt.Errorf("待校验的旧记忆不存在或没有正文")
	}

	model := currentEmbeddingModel()
	sources := make([][]float32, 0, len(docs))
	for _, doc := range docs {
		vec := doc.Embedding
		if len(vec) == 0 {
			embedded, embedErr := d.embedText(ctx, model, doc.Content)
			if embedErr != nil {
				return fmt.Errorf("来源记忆 %s 向量缺失且无法重建: %w", doc.ID, embedErr)
			}
			vec = embedded
		}
		sources = append(sources, vec)
	}
	newVectors, err := d.embedTexts(ctx, model, texts)
	if err != nil {
		return fmt.Errorf("整理结果嵌入失败: %w", err)
	}

	dimension := len(sources[0])
	alignedSources := sources[:0]
	for _, vec := range sources {
		if len(vec) != dimension {
			continue
		}
		alignedSources = append(alignedSources, vec)
	}
	if len(alignedSources) == 0 {
		return fmt.Errorf("来源记忆向量维度不一致，无法校验")
	}
	threshold = normalizeClusterThreshold(threshold)

	for index, vec := range newVectors {
		if len(vec) != dimension {
			return fmt.Errorf("整理结果第 %d 条向量维度与来源记忆不一致", index+1)
		}
		best := maxCosine(vec, alignedSources)
		if best < threshold {
			return fmt.Errorf("整理结果第 %d 条与来源记忆相似度 %.3f 低于校验门槛 %.2f", index+1, best, threshold)
		}
	}
	for index, source := range alignedSources {
		best := maxCosine(source, newVectors)
		if best < threshold {
			return fmt.Errorf("来源记忆第 %d 条未被整理结果覆盖（最高相似度 %.3f 低于 %.2f）", index+1, best, threshold)
		}
	}
	return nil
}

// collectTextCandidates 取候选文本条目：跳过媒体/空正文；ids 非空时只取命中的条目。
// 结果按入库时间升序排序（缺失时间戳的旧数据按最久远处理），保证聚类代表稳定。
func (d *Store) collectTextCandidates(collectionName string, ids []string) ([]Document, int, error) {
	var collections []*Collection
	if strings.TrimSpace(collectionName) != "" {
		c, err := d.getCollection(collectionName)
		if err != nil {
			return nil, 0, err
		}
		collections = []*Collection{c}
	} else {
		collections = d.snapshotCollections()
	}

	wanted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			wanted[trimmed] = true
		}
	}

	out := make([]Document, 0)
	skipped := 0
	for _, c := range collections {
		c.reloadIfChanged()
		c.mu.RLock()
		docs := append([]Document(nil), c.Documents...)
		c.mu.RUnlock()
		for _, doc := range docs {
			if len(wanted) > 0 && !wanted[doc.ID] {
				continue
			}
			if documentType(doc) != TypeText || strings.TrimSpace(doc.Content) == "" || doc.URL != "" || doc.Base64 != "" {
				skipped++
				continue
			}
			out = append(out, doc)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	return out, skipped, nil
}

// clusterTextDocs 代表式聚类：与已有簇代表条目相似度达阈值即并入最相似的簇。
// maxItems/maxRunes 限制单簇规模，超过时另起一簇，避免弱相关条目链式串联。
func clusterTextDocs(docs []Document, threshold float32, maxItems, maxRunes int) []fuseCluster {
	clusters := make([]fuseCluster, 0)
	runes := make([]int, 0)
	for i, doc := range docs {
		contentRunes := len([]rune(doc.Content))
		bestIndex := -1
		bestSim := float32(0)
		for ci := range clusters {
			if maxItems > 0 && len(clusters[ci].indices) >= maxItems {
				continue
			}
			if maxRunes > 0 && runes[ci]+contentRunes > maxRunes {
				continue
			}
			rep := docs[clusters[ci].indices[0]]
			sim := cosineSimilarity(doc.Embedding, rep.Embedding)
			if sim >= threshold && sim > bestSim {
				bestSim = sim
				bestIndex = ci
			}
		}
		if bestIndex >= 0 {
			clusters[bestIndex].indices = append(clusters[bestIndex].indices, i)
			runes[bestIndex] += contentRunes
			if bestSim < clusters[bestIndex].minSimilarity {
				clusters[bestIndex].minSimilarity = bestSim
			}
			continue
		}
		clusters = append(clusters, fuseCluster{indices: []int{i}, minSimilarity: 1})
		runes = append(runes, contentRunes)
	}
	return clusters
}

func normalizeClusterThreshold(value float32) float32 {
	if value <= 0 || value < ClusterMinThreshold || value > 1 {
		return ClusterDefaultThreshold
	}
	return value
}

func normalizeTextList(texts []string) []string {
	out := make([]string, 0, len(texts))
	for _, text := range texts {
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func maxCosine(vector []float32, candidates [][]float32) float32 {
	best := float32(0)
	for _, candidate := range candidates {
		if len(candidate) != len(vector) {
			continue
		}
		if sim := cosineSimilarity(vector, candidate); sim > best {
			best = sim
		}
	}
	return best
}
