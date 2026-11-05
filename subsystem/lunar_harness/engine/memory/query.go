package memory

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Query 在所有存储单元（总库 + 正式簇）中检索指定类型的记忆。
//
// 默认走「簇质心路由」：先算查询与每个正式簇质心的余弦，低于 ClusterCentroidGate
// 的簇直接跳过，其余按相似度降序取前 QueryTopClusters 个；只在命中簇的成员里按
// ClusterMemberGate 过滤后取 topK。结果不足 topK 时回落扫描总库与未被路由命中的簇
// （排除已返回的 ID），补齐到 topK 或扫完为止。
//
// 没有任何簇通过质心门限、或索引为空时，退化为全库扫描（等价暴力检索）。
// ForceGlobal 打开时始终全库扫描。实际比较条目数由 QueryWithStats 的返回值给出。
func (d *Store) Query(ctx context.Context, queryText, mediaType string, topK int, opts QueryOptions) ([]Entry, error) {
	results, _, err := d.QueryWithStats(ctx, queryText, mediaType, topK, opts)
	return results, err
}

// QueryWithStats 与 Query 相同，同时返回本次路由的观测数据（实际比较条目数等）。
func (d *Store) QueryWithStats(ctx context.Context, queryText, mediaType string, topK int, opts QueryOptions) ([]Entry, QueryStats, error) {
	stats := QueryStats{ForceGlobal: opts.ForceGlobal}
	if topK <= 0 {
		return nil, stats, nil
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, stats, nil
	}
	mediaType, err := NormalizeType(mediaType)
	if err != nil {
		return nil, stats, err
	}

	collections := d.snapshotCollections()
	if len(collections) == 0 {
		return nil, stats, nil
	}
	for _, c := range collections {
		c.reloadIfChanged()
	}

	model := currentEmbeddingModel()
	queryVec, embedErr := d.embedQuery(ctx, model, queryText)
	if embedErr != nil {
		logger.Warn("FileManager", "记忆检索：查询嵌入失败（模型 %s）: %v", model, embedErr)
		return nil, stats, fmt.Errorf("查询嵌入失败: %w", embedErr)
	}
	if len(queryVec) == 0 {
		return nil, stats, nil
	}

	index := d.cloneClusterIndex()

	// ===== 退化路径：强制全局 / 索引缺失 / 无正式簇 → 全库扫描 =====
	if opts.ForceGlobal || index == nil || len(index.Clusters) == 0 {
		results, compared := scanAllCollections(collections, queryVec, mediaType, nil, DocRankThreshold)
		stats.DocsCompared = compared
		stats.FallbackDocs = compared
		stats.UsedFallback = true
		return finalizeTopK(results, topK), stats, nil
	}

	// ===== 1. 质心路由：按 cos(q, centroid) 排序，低于门限跳过，取前 QueryTopClusters =====
	stats.ClustersTotal = len(index.Clusters)
	admitted := rankClustersByCentroid(index.Clusters, queryVec, &stats)
	stats.ClustersHit = len(admitted)

	if len(admitted) == 0 {
		// 没有任何簇通过质心门限：直接全库扫描（总库 + 全部簇）
		results, compared := scanAllCollections(collections, queryVec, mediaType, nil, DocRankThreshold)
		stats.DocsCompared = compared
		stats.FallbackDocs = compared
		stats.UsedFallback = true
		logger.Info("FileManager", "记忆检索：%d 个簇均未通过质心门限（%.2f），回落全库扫描",
			stats.ClustersCompared, float64(ClusterCentroidGate))
		return finalizeTopK(results, topK), stats, nil
	}

	// ===== 2. 簇内检索：只在命中簇的成员里比较，低于成员门限跳过 =====
	byName := make(map[string]*Collection, len(collections))
	for _, c := range collections {
		byName[c.Name] = c
	}

	var candidates []Entry
	compared := 0
	for _, hit := range admitted {
		collection, ok := byName[hit.node.ID]
		if !ok {
			continue // 索引引用的簇目录尚未加载（迁移中），跳过
		}
		memberSet := make(map[string]bool, len(hit.node.DocIDs))
		for _, id := range hit.node.DocIDs {
			memberSet[id] = true
		}
		entries, n := collection.scoreDocs(queryVec, mediaType, memberSet, nil, ClusterMemberGate)
		compared += n
		candidates = append(candidates, entries...)
	}
	stats.ClusterDocs = compared
	stats.DocsCompared = compared

	results := finalizeTopK(candidates, topK)
	if len(results) >= topK {
		return results, stats, nil
	}

	// ===== 3. 回落补齐：扫描总库与未被路由命中的簇，排除已返回 ID =====
	stats.UsedFallback = true
	exclude := make(map[string]bool, len(results))
	for _, r := range results {
		exclude[r.ID] = true
	}
	extra, n := scanAllCollections(collections, queryVec, mediaType, exclude, DocRankThreshold)
	stats.FallbackDocs = n
	stats.DocsCompared += n
	results = append(results, extra...)
	logger.Info("FileManager", "记忆检索：簇路由仅得 %d/%d 条，回落补齐比较 %d 条",
		len(exclude), topK, n)
	return finalizeTopK(results, topK), stats, nil
}

// rankClustersByCentroid 计算查询与各正式簇质心的余弦，过滤低于 ClusterCentroidGate 的簇，
// 按相似度降序（同分按簇 ID 升序，保证可复现）截取前 QueryTopClusters 个。
func rankClustersByCentroid(nodes []ClusterNode, queryVec []float32, stats *QueryStats) []clusterRank {
	ranked := make([]clusterRank, 0, len(nodes))
	for _, node := range nodes {
		if len(node.Centroid) == 0 || len(node.Centroid) != len(queryVec) {
			continue // 维度不符：不参与路由（实体条目仍可由回落覆盖）
		}
		stats.ClustersCompared++
		sim := cosineSimilarity(queryVec, node.Centroid)
		if sim < ClusterCentroidGate {
			continue
		}
		ranked = append(ranked, clusterRank{node: node, similarity: sim})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].similarity != ranked[j].similarity {
			return ranked[i].similarity > ranked[j].similarity
		}
		return ranked[i].node.ID < ranked[j].node.ID
	})
	if len(ranked) > QueryTopClusters {
		ranked = ranked[:QueryTopClusters]
	}
	return ranked
}

// clusterRank 一个通过质心门限的簇及其与查询的相似度。
type clusterRank struct {
	node       ClusterNode
	similarity float32
}

// scanAllCollections 扫描给定集合的全部条目（可排除已返回 ID），返回全部达标候选与比较次数。
//
// 它是"全库扫描"的唯一实现：ForceGlobal、无簇命中退化、以及回落补齐都复用它，
// 因此回落阶段天然覆盖总库 + 全部簇成员，不会漏掉未被路由命中的簇条目。
func scanAllCollections(collections []*Collection, queryVec []float32, mediaType string, exclude map[string]bool, minSim float32) ([]Entry, int) {
	var results []Entry
	compared := 0
	for _, c := range collections {
		entries, n := c.scoreDocs(queryVec, mediaType, nil, exclude, minSim)
		compared += n
		results = append(results, entries...)
	}
	return results, compared
}

// scoreDocs 在集合内按过滤条件逐条计算余弦，返回达标条目与"实际比较条目数"。
//
// 比较次数只统计真正做了 cosineSimilarity 的条目：维度不符、类型不符、Pending、
// 不在成员集合、已在排除集合里的条目都不计入，从而准确反映路由省下的计算量。
func (c *Collection) scoreDocs(queryVec []float32, mediaType string, memberSet, exclude map[string]bool, minSim float32) ([]Entry, int) {
	if len(queryVec) == 0 {
		return nil, 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	compared := 0
	results := make([]Entry, 0, len(c.Documents))
	for i := range c.Documents {
		doc := &c.Documents[i]
		if len(doc.Embedding) == 0 || len(doc.Embedding) != len(queryVec) {
			continue
		}
		if documentType(*doc) != mediaType {
			continue
		}
		// 待处理条目只对整理可见：检索默认不返回，避免同一事实出现两遍
		if doc.Pending {
			continue
		}
		if memberSet != nil && !memberSet[doc.ID] {
			continue
		}
		if exclude != nil && exclude[doc.ID] {
			continue
		}
		compared++
		sim := cosineSimilarity(queryVec, doc.Embedding)
		if sim < minSim {
			continue
		}
		results = append(results, resultOf(c, doc, sim))
	}
	return results, compared
}

// finalizeTopK 按相似度降序（同分按入库时间倒序、ID 升序）排序并截取前 topK 条。
func finalizeTopK(results []Entry, topK int) []Entry {
	if len(results) == 0 || topK <= 0 {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Similarity != results[j].Similarity {
			return results[i].Similarity > results[j].Similarity
		}
		if results[i].Timestamp != results[j].Timestamp {
			return results[i].Timestamp > results[j].Timestamp
		}
		return results[i].ID < results[j].ID
	})
	if topK > len(results) {
		topK = len(results)
	}
	return results[:topK]
}

// KeywordSearch 在全部集合中按关键词或正则扫描文本记忆（不调用嵌入服务）
//
// 与向量检索互补：
//   - 关键词模式：terms 全部命中（大小写不敏感的子串匹配），适合找人名、原话、专有名词；
//   - 正则模式：terms 以空格拼接后作为正则表达式（自动 (?i) 忽略大小写）
//     适合精确模式匹配。Go regexp RE2 语法，不支持反向引用与环视
//
// 返回按入库时间倒序的结果；Similarity 固定 0，由调用方用检索方式区分展示
func (d *Store) KeywordSearch(mediaType string, terms []string, useRegex bool, limit int) ([]Entry, error) {
	if limit <= 0 || len(terms) == 0 {
		return nil, nil
	}
	mediaType, err := NormalizeType(mediaType)
	if err != nil {
		return nil, err
	}

	var match func(string) bool
	if useRegex {
		re, err := regexp.Compile("(?i)" + strings.Join(terms, " "))
		if err != nil {
			return nil, fmt.Errorf("正则表达式无效: %w", err)
		}
		match = re.MatchString
	} else {
		lowered := make([]string, 0, len(terms))
		for _, term := range terms {
			if text := strings.ToLower(strings.TrimSpace(term)); text != "" {
				lowered = append(lowered, text)
			}
		}
		if len(lowered) == 0 {
			return nil, nil
		}
		match = func(text string) bool {
			lower := strings.ToLower(text)
			for _, term := range lowered {
				if !strings.Contains(lower, term) {
					return false
				}
			}
			return true
		}
	}

	var results []Entry
	for _, c := range d.snapshotCollections() {
		c.reloadIfChanged()
		results = append(results, c.keywordResults(match, mediaType)...)
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Timestamp > results[j].Timestamp
	})
	if limit > len(results) {
		limit = len(results)
	}
	return results[:limit], nil
}

// keywordResults 在当前集合内按匹配函数扫描文本记忆
func (c *Collection) keywordResults(match func(string) bool, mediaType string) []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	results := make([]Entry, 0)
	for i := len(c.Documents) - 1; i >= 0; i-- {
		doc := c.Documents[i]
		if documentType(doc) != mediaType {
			continue
		}
		// 待处理条目只对整理可见：检索默认不返回，避免同一事实出现两遍
		if doc.Pending {
			continue
		}
		// 媒体记忆没有正文，不参与关键词检索
		if doc.Content == "" || !match(doc.Content) {
			continue
		}
		results = append(results, resultOf(c, &doc, 0))
	}
	return results
}
