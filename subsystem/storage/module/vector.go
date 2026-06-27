package module

import (
	"math"
	"sort"
)

// cosineSimilarity 计算两个向量的余弦相似度，取值范围 [-1, 1]，越高越相似
// 当向量长度不一致或为零向量时返回 0
func cosineSimilarity(a, b []float32) float32 {
	n := len(a)
	if n != len(b) || n == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := 0; i < n; i++ {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

// queryTopK 按 queryVec 检索最相似的 topK 篇文档，按相似度降序返回
// 相似度相同的文档保持其在 Documents 中的原始顺序（稳定排序）
func (c *Collection) queryTopK(queryVec []float32, topK int) []VectorQueryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Documents) == 0 || topK <= 0 {
		return nil
	}

	type scored struct {
		index int
		score float32
	}
	results := make([]scored, 0, len(c.Documents))
	for i := range c.Documents {
		results = append(results, scored{
			index: i,
			score: cosineSimilarity(queryVec, c.Documents[i].Embedding),
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if topK > len(results) {
		topK = len(results)
	}

	out := make([]VectorQueryResult, topK)
	for i := 0; i < topK; i++ {
		doc := &c.Documents[results[i].index]
		out[i] = VectorQueryResult{
			ID:         doc.ID,
			Role:       doc.Role,
			Content:    doc.Content,
			Similarity: results[i].score,
		}
	}
	return out
}
