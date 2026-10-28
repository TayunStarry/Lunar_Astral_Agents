package memory

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Query 在所有集合中检索指定类型的记忆，并按选项选择检索范围。
//
// 快速检索（默认）把「集合名向量」当作目录索引：先查与查询含义最接近的集合；
// 命中则补查第 2~5 近的集合，首集合未命中则回退扫描其余全部集合。
// 这为将来集合数量增长留出扩展空间——查询成本先随「最相近的几个集合」走，
// 而不是一上来就对全部集合做全量向量比较。
//
// ForceGlobal 打开时跳过这层路由，始终扫描全部集合（结果最全，成本最高）。
func (d *Store) Query(ctx context.Context, queryText, mediaType string, topK int, opts QueryOptions) ([]Entry, error) {
	if topK <= 0 {
		return nil, nil
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, nil
	}
	mediaType, err := NormalizeType(mediaType)
	if err != nil {
		return nil, err
	}

	collections := d.snapshotCollections()
	if len(collections) == 0 {
		return nil, nil
	}

	// 1. 每个嵌入模型只嵌入一次查询文本；嵌入失败的模型本轮不参与检索
	queryVectors := make(map[string][]float32)
	var lastErr error
	usable := make([]*Collection, 0, len(collections))
	for _, c := range collections {
		c.reloadIfChanged()
		model := currentEmbeddingModel()
		vec, ok := queryVectors[model]
		if !ok {
			embedded, embedErr := d.embedQuery(ctx, model, queryText)
			if embedErr != nil {
				lastErr = embedErr
				logger.Warn("FileManager", "集合 [%s] 查询嵌入失败（模型 %s）: %v", c.Name, model, embedErr)
				continue
			}
			queryVectors[model] = embedded
			vec = embedded
		}
		if len(vec) == 0 {
			continue
		}
		usable = append(usable, c)
	}
	if len(usable) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("查询嵌入失败: %w", lastErr)
		}
		return nil, nil
	}

	// 2. 按「查询向量 ↔ 集合名向量」余弦相似度排序（同名/同分按集合名兜底，保证可复现）
	ordered, ranked := rankCollectionsByQuery(usable, queryVectors)

	// 3. 决定本轮扫描范围
	var results []Entry
	switch {
	case opts.ForceGlobal || !ranked:
		// 强制全局，或没有任何集合名向量可比（退化）：扫描全部集合
		results = scanCollectionsTopK(usable, queryVectors, mediaType, topK)
	default:
		first := ordered[0]
		results = scanCollectionsTopK([]*Collection{first}, queryVectors, mediaType, topK)
		if len(results) > 0 {
			// 首集合命中：补查第 2~5 近的集合，避免只得到单一视角的答案
			limit := len(ordered)
			if limit > FastSearchCollectionLimit {
				limit = FastSearchCollectionLimit
			}
			if limit > 1 {
				results = append(results, scanCollectionsTopK(ordered[1:limit], queryVectors, mediaType, topK)...)
			}
		} else {
			// 首集合没有达到门槛的结果：回退扫描其余全部集合，保证召回不因路由而丢失
			if len(ordered) > 1 {
				results = scanCollectionsTopK(ordered[1:], queryVectors, mediaType, topK)
			}
			if len(results) > 0 {
				logger.Info("FileManager", "记忆快速检索：最相近集合 [%s] 未命中，已回退扫描其余 %d 个集合",
					first.Name, len(ordered)-1)
			}
		}
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Similarity != results[j].Similarity {
			return results[i].Similarity > results[j].Similarity
		}
		return results[i].Timestamp > results[j].Timestamp
	})
	if topK > len(results) {
		topK = len(results)
	}
	return results[:topK], nil
}

// rankCollectionsByQuery 按集合名向量与查询向量的余弦相似度给集合排序（降序）。
//
// 返回 ordered（全部集合的排序结果）与 ranked（是否存在可比较的集合名向量）。
// 没有集合名向量/维度不符的集合排在最后，仅在回退或强制全局时才会被扫描。
func rankCollectionsByQuery(collections []*Collection, queryVectors map[string][]float32) ([]*Collection, bool) {
	type scoredCollection struct {
		collection *Collection
		similarity float32
		comparable bool
	}

	scored := make([]scoredCollection, 0, len(collections))
	ranked := false
	for _, c := range collections {
		c.mu.RLock()
		nameEmbedding := append([]float32(nil), c.NameEmbedding...)
		c.mu.RUnlock()

		queryVec := queryVectors[currentEmbeddingModel()]
		comparable := len(queryVec) > 0 && len(nameEmbedding) == len(queryVec)
		similarity := float32(-2)
		if comparable {
			similarity = cosineSimilarity(queryVec, nameEmbedding)
			ranked = true
		}
		scored = append(scored, scoredCollection{collection: c, similarity: similarity, comparable: comparable})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].comparable != scored[j].comparable {
			return scored[i].comparable
		}
		if scored[i].similarity != scored[j].similarity {
			return scored[i].similarity > scored[j].similarity
		}
		return scored[i].collection.Name < scored[j].collection.Name
	})

	ordered := make([]*Collection, len(scored))
	for i := range scored {
		ordered[i] = scored[i].collection
	}
	return ordered, ranked
}

// scanCollectionsTopK 在给定集合内做内容向量检索并汇总结果（调用方负责排序与截断）
func scanCollectionsTopK(collections []*Collection, queryVectors map[string][]float32, mediaType string, topK int) []Entry {
	var results []Entry
	for _, c := range collections {
		vec := queryVectors[currentEmbeddingModel()]
		if len(vec) == 0 {
			continue
		}
		results = append(results, c.queryTopK(vec, topK, mediaType)...)
	}
	return results
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
		// 媒体记忆没有正文，不参与关键词检索
		if doc.Content == "" || !match(doc.Content) {
			continue
		}
		results = append(results, resultOf(c, &doc, 0))
	}
	return results
}

// queryTopK 内容向量检索核心算法
// 嵌入查询与全量文档内容向量计算余弦相似度，按类型过滤，低于 DocRankThreshold 丢弃
// 按相似度降序（同分按插入顺序）返回 topK
func (c *Collection) queryTopK(queryVec []float32, topK int, mediaType string) []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Documents) == 0 || topK <= 0 {
		return nil
	}

	type ranked struct {
		idx   int
		sim   float32
		order int
	}
	rankedList := make([]ranked, 0, len(c.Documents))
	for i := range c.Documents {
		doc := &c.Documents[i]
		if len(doc.Embedding) == 0 || documentType(*doc) != mediaType {
			continue
		}
		sim := cosineSimilarity(queryVec, doc.Embedding)
		if sim < DocRankThreshold {
			continue
		}
		rankedList = append(rankedList, ranked{idx: i, sim: sim, order: i})
	}

	if len(rankedList) == 0 {
		return nil
	}

	sort.SliceStable(rankedList, func(i, j int) bool {
		if rankedList[i].sim != rankedList[j].sim {
			return rankedList[i].sim > rankedList[j].sim
		}
		return rankedList[i].order < rankedList[j].order
	})

	if topK > len(rankedList) {
		topK = len(rankedList)
	}

	results := make([]Entry, topK)
	for i := 0; i < topK; i++ {
		results[i] = resultOf(c, &c.Documents[rankedList[i].idx], rankedList[i].sim)
	}
	return results
}
