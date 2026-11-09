package wishchain

// match.go 创作 / 观影结果与愿望的文本匹配。
//
// 愿望没有显式分类标签，因此用「愿望内容在结果文本中的字面覆盖率」做匹配：
// 结果文本可以包含标题、简介、主题词、创作选题与活动文案；匹配到最相关的一条
// 待实现愿望后，完成或强制逾期。匹配不到时不做任何改动，避免误伤无关愿望。

import (
	"strings"
	"time"
	"unicode"
)

// matchThreshold 愿望内容的二元组在结果文本中的最低覆盖率。
const matchThreshold = 0.15

// CompleteMatching 找到与文本最匹配的待实现愿望并标记完成。
func CompleteMatching(text string) (Wish, bool) {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := matchPendingLocked(text)
	if index < 0 {
		return Wish{}, false
	}
	item := &state.Wishes[index]
	if item.Status == StatusCompleted {
		return *item, false
	}
	now := time.Now().Unix()
	item.Status = StatusCompleted
	item.CompletedAt = now
	item.UpdatedAt = now
	item.CompletedSettled = false
	saveLocked()
	return *item, true
}

// MarkOverdueMatching 找到与文本最匹配的待实现愿望并标记为逾期。
//
// 逾期标记只负责状态；心情联动仍由生活链固定步骤的 Settle 统一结算。
func MarkOverdueMatching(text string) (Wish, bool) {
	mu.Lock()
	defer mu.Unlock()
	ensureLoadedLocked()
	index := matchPendingLocked(text)
	if index < 0 {
		return Wish{}, false
	}
	item := &state.Wishes[index]
	if item.ForcedOverdue {
		return *item, false
	}
	item.ForcedOverdue = true
	item.OverdueSettled = false
	item.UpdatedAt = time.Now().Unix()
	saveLocked()
	return *item, true
}

// matchPendingLocked 选出与文本最相关的待实现愿望；没有达到阈值时返回 -1。
// 同分时优先更高优先级（数字更小）。
func matchPendingLocked(text string) int {
	target := normalizeMatchText(text)
	if target == "" {
		return -1
	}
	best := -1
	bestScore := 0.0
	bestPriority := MaxPriority + 1
	for index := range state.Wishes {
		item := &state.Wishes[index]
		if item.Status != StatusPending {
			continue
		}
		score := matchScore(item.Content, target)
		if score < matchThreshold {
			continue
		}
		if score > bestScore || (score == bestScore && item.Priority < bestPriority) {
			best = index
			bestScore = score
			bestPriority = item.Priority
		}
	}
	return best
}

// matchScore 计算愿望内容在结果文本中的覆盖率。
//
// 先看完整子串；否则用字符二元组覆盖率，避免长文本把分数稀释掉。
func matchScore(content, target string) float64 {
	wish := normalizeMatchText(content)
	if wish == "" || target == "" {
		return 0
	}
	if len([]rune(wish)) >= 2 && strings.Contains(target, wish) {
		return 1
	}
	wishGrams := bigrams(wish)
	if len(wishGrams) == 0 {
		return 0
	}
	targetGrams := bigrams(target)
	common := 0
	for gram := range wishGrams {
		if targetGrams[gram] {
			common++
		}
	}
	return float64(common) / float64(len(wishGrams))
}

// normalizeMatchText 只保留文字与数字，去掉标点、空白与书名号等噪声。
func normalizeMatchText(text string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(text) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

// bigrams 生成字符二元组集合。
func bigrams(text string) map[string]bool {
	runes := []rune(text)
	if len(runes) < 2 {
		return nil
	}
	out := make(map[string]bool, len(runes)-1)
	for i := 0; i+1 < len(runes); i++ {
		out[string(runes[i:i+2])] = true
	}
	return out
}
