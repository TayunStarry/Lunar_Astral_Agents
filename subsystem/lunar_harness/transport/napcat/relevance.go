package napcat

// relevance.go 随机触发时的「自动引用最相关消息」
//
// 群聊里随机触发没有 @、也没有关键词，一条回应发出去，群里看不出月华在接谁的话。
// 这里用嵌入模型把「本批次每条消息」与「月华的实际回应」做相关性比较，取最高的一条
// 作为 reply 段引用，用户点开引用即可看到被回应的原消息。
//
// 失败兜底：嵌入服务不可用/维度不一致时，退回引用批次里最新的一条（最可能是刚说完的那句）。

import (
	"context"
	"math"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
)

// quoteEmbeddingTimeout 相关性计算的总超时：超时就按「最新一条」兜底，不阻塞回复发送
const quoteEmbeddingTimeout = 8 * time.Second

// pickQuoteMessageID 在候选消息中挑出与回应最相关的一条，返回其消息 ID（0 表示不引用）
func pickQuoteMessageID(reply string, candidates []QuoteCandidate) int64 {
	filtered := make([]QuoteCandidate, 0, len(candidates))
	for _, item := range candidates {
		if item.MessageID == 0 {
			continue
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return 0
	}
	// 只有一条消息时无需计算：随机触发也只可能是在接这一条
	if len(filtered) == 1 {
		return filtered[0].MessageID
	}
	text := strings.TrimSpace(reply)
	if text == "" {
		return filtered[len(filtered)-1].MessageID
	}

	texts := make([]string, len(filtered))
	for i, item := range filtered {
		texts[i] = strings.TrimSpace(item.Content)
	}
	ctx, cancel := context.WithTimeout(context.Background(), quoteEmbeddingTimeout)
	defer cancel()

	documents, err := persistence.EmbedDocumentTexts(ctx, texts)
	if err != nil || len(documents) != len(filtered) {
		logger.SubWarn("LunarCore", "Napcat", "随机触发引用：消息向量计算失败，改引用最新一条（%v）", err)
		return filtered[len(filtered)-1].MessageID
	}
	query, err := persistence.EmbedQueryText(ctx, text)
	if err != nil || len(query) == 0 {
		logger.SubWarn("LunarCore", "Napcat", "随机触发引用：回应向量计算失败，改引用最新一条（%v）", err)
		return filtered[len(filtered)-1].MessageID
	}

	bestIndex := -1
	bestScore := math.Inf(-1)
	for i, vector := range documents {
		score := cosineSimilarity(query, vector)
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return filtered[len(filtered)-1].MessageID
	}
	logger.SubInfo("LunarCore", "Napcat", "随机触发引用：选中消息 %d（相似度 %.3f）",
		filtered[bestIndex].MessageID, bestScore)
	return filtered[bestIndex].MessageID
}

// buildQuoteCandidates 从批次条目中提取可引用的消息。
//
// 优先取有文本内容的消息（这样才能算相关性）；整批都是纯图片/纯媒体时，
// 退回引用最新一条——纯图片消息同样值得被引用，「这张图」本身就是回应对象。
func buildQuoteCandidates(entries []GroupPoolEntry) []QuoteCandidate {
	all := make([]QuoteCandidate, 0, len(entries))
	withText := make([]QuoteCandidate, 0, len(entries))
	for _, entry := range entries {
		if entry.MessageID == 0 {
			continue
		}
		candidate := QuoteCandidate{
			MessageID: entry.MessageID,
			Content:   strings.TrimSpace(contentToText(entry.Content)),
		}
		all = append(all, candidate)
		if candidate.Content != "" {
			withText = append(withText, candidate)
		}
	}
	if len(withText) > 0 {
		return withText
	}
	if len(all) > 0 {
		return all[len(all)-1:]
	}
	return nil
}

// cosineSimilarity 计算两个向量的余弦相似度（维度不一致时返回 -1）
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return -1
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return -1
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
