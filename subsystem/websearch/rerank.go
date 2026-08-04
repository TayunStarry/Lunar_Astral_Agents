package websearch

import (
	"math"
	"sort"
	"strings"
)

// EmbeddingProvider 向量化接口，用于计算文本相似度进行重排序
type EmbeddingProvider interface {
	// GetEmbedding 获取文本的向量表示
	GetEmbedding(text string) ([]float64, error)
}

// Reranker 重排序器
type Reranker struct {
	embedding EmbeddingProvider
	debugLog  func(format string, args ...interface{})
}

// NewReranker 创建重排序器
func NewReranker(embedding EmbeddingProvider, debugLog func(format string, args ...interface{})) *Reranker {
	return &Reranker{
		embedding: embedding,
		debugLog:  debugLog,
	}
}

// RerankConfig 重排序配置
type RerankConfig struct {
	MinResults int // 最小结果数阈值，低于此值不触发重排序（默认5）
	MaxResults int // 最大重排序结果数（默认20）
	TopN       int // 返回前N条结果（0表示全部）
}

// DefaultRerankConfig 默认重排序配置
func DefaultRerankConfig() RerankConfig {
	return RerankConfig{
		MinResults: 5,
		MaxResults: 20,
		TopN:       0,
	}
}

// Rerank 对搜索结果按余弦相似度重排序
func (r *Reranker) Rerank(query string, results []SearchResult, cfg RerankConfig) []SearchResult {
	if r.embedding == nil {
		if r.debugLog != nil {
			r.debugLog("[Rerank] 向量化提供者未配置，跳过多余的重排序")
		}
		return results
	}

	if len(results) <= cfg.MinResults {
		if r.debugLog != nil {
			r.debugLog("[Rerank] 结果数=%d <= 阈值=%d，跳过重排序", len(results), cfg.MinResults)
		}
		return results
	}

	// 限制最大结果数
	toRerank := results
	if len(toRerank) > cfg.MaxResults {
		toRerank = toRerank[:cfg.MaxResults]
	}

	if r.debugLog != nil {
		r.debugLog("[Rerank] 开始重排序 query=%q 结果数=%d", query, len(toRerank))
	}

	// 获取查询向量
	queryVec, err := r.embedding.GetEmbedding(query)
	if err != nil {
		if r.debugLog != nil {
			r.debugLog("[Rerank] 获取查询向量失败: %v，跳过重排序", err)
		}
		return results
	}

	// 计算每条结果的相似度
	type scoredResult struct {
		index int
		score float64
	}
	scored := make([]scoredResult, len(toRerank))

	for i, res := range toRerank {
		// 拼接标题和摘要作为文档文本
		docText := buildDocText(res)
		docVec, err := r.embedding.GetEmbedding(docText)
		if err != nil {
			if r.debugLog != nil {
				r.debugLog("[Rerank] 获取文档向量失败 [%d] %s: %v", i, res.Title, err)
			}
			scored[i] = scoredResult{index: i, score: 0}
			continue
		}
		scored[i] = scoredResult{
			index: i,
			score: cosineSimilarity(queryVec, docVec),
		}
	}

	// 按相似度降序排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// 构建重排序后的结果
	reranked := make([]SearchResult, 0, len(toRerank))
	for _, s := range scored {
		reranked = append(reranked, toRerank[s.index])
	}

	if r.debugLog != nil {
		r.debugLog("[Rerank] 重排序完成 最高分=%.4f 最低分=%.4f",
			scored[0].score, scored[len(scored)-1].score)
	}

	// 如果指定了 TopN，截取前 N 条
	if cfg.TopN > 0 && len(reranked) > cfg.TopN {
		reranked = reranked[:cfg.TopN]
	}

	// 保留未被重排序的结果（超过 MaxResults 的部分）
	if len(results) > cfg.MaxResults {
		reranked = append(reranked, results[cfg.MaxResults:]...)
	}

	return reranked
}

// buildDocText 构建用于向量化的文档文本
// 拼接标题和摘要，优先使用标题
func buildDocText(res SearchResult) string {
	var sb strings.Builder
	if res.Title != "" {
		sb.WriteString(res.Title)
	}
	if res.Snippet != "" {
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(res.Snippet)
	}
	return sb.String()
}

// cosineSimilarity 计算两个向量的余弦相似度
// 返回值范围 [-1, 1]，越接近1表示越相似
func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	// 确保向量长度一致
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	var dotProduct, normA, normB float64
	for i := 0; i < minLen; i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// applyRerank 对搜索结果应用余弦相似度重排序
func applyRerank(reranker *Reranker, query string, results []SearchResult, _ func(format string, args ...interface{})) []SearchResult {
	if reranker == nil {
		return results
	}
	cfg := DefaultRerankConfig()
	return reranker.Rerank(query, results, cfg)
}
