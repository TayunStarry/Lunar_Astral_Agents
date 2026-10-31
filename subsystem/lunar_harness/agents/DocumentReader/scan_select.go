package DocumentReader

// scan_select.go 轻量扫描的选片：片数超过单次上限时，按需求相关度抽取，
// 并保留开头与结尾，避免只扫描到文件前部。

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// selectScanSpans 选出本次要扫描的分片；不超过上限时返回全部分片
func selectScanSpans(doc *docContext, chunks []chunkSpan, plan scanPlan, max int) []chunkSpan {
	if len(chunks) == 0 || max <= 0 {
		return nil
	}
	if len(chunks) <= max {
		return chunks
	}
	terms := scanTerms(plan.Goal, plan.Focus)
	scored := make([]spanScore, 0, len(chunks))
	for _, span := range chunks {
		scored = append(scored, spanScore{Span: span, Score: scoreSpan(doc, span, terms)})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].Span.Index < scored[j].Span.Index
		}
		return scored[i].Score > scored[j].Score
	})
	if scored[0].Score == 0 {
		return sampleSpans(chunks, max)
	}

	picked := make([]chunkSpan, 0, max)
	seen := map[int]bool{}
	appendSpan := func(span chunkSpan) {
		if len(picked) >= max || seen[span.Index] {
			return
		}
		seen[span.Index] = true
		picked = append(picked, span)
	}
	// 开头与结尾保留，便于把握全文起止
	appendSpan(chunks[0])
	if len(chunks) > 1 {
		appendSpan(chunks[len(chunks)-1])
	}
	for _, item := range scored {
		appendSpan(item.Span)
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].Index < picked[j].Index })
	return picked
}

// scanTerms 从需求与关注词中提取检索词；中文长串补二元/三元词
func scanTerms(goal string, focus []string) []string {
	terms := make([]string, 0, 64)
	seen := map[string]bool{}
	add := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if len([]rune(term)) < 2 || seen[term] {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	appendField := func(field string) {
		runes := []rune(field)
		if len(runes) <= 6 {
			add(field)
			return
		}
		add(field)
		if containsHan(runes) {
			for i := 0; i+1 < len(runes); i++ {
				add(string(runes[i : i+2]))
			}
			for i := 0; i+2 < len(runes); i++ {
				add(string(runes[i : i+3]))
			}
		}
	}
	for _, field := range splitTerms(goal) {
		appendField(field)
	}
	for _, item := range focus {
		for _, field := range splitTerms(item) {
			appendField(field)
		}
	}
	if len(terms) > 80 {
		terms = terms[:80]
	}
	return terms
}

// splitTerms 把文本按非字母数字字符切开
func splitTerms(text string) []string {
	var builder strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		} else {
			builder.WriteRune(' ')
		}
	}
	return strings.Fields(builder.String())
}

// containsHan 判断字符片段是否含中文
func containsHan(runes []rune) bool {
	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// scoreSpan 计算分片相关度：词频 + 大纲标题命中加权
func scoreSpan(doc *docContext, span chunkSpan, terms []string) int {
	start, end := span.StartLine, span.EndLine
	if start < 1 {
		start = 1
	}
	if end > len(doc.Lines) {
		end = len(doc.Lines)
	}
	if start > end {
		return 0
	}
	text := strings.ToLower(strings.Join(doc.Lines[start-1:end], "\n"))
	score := 0
	for _, term := range terms {
		score += strings.Count(text, term)
	}
	for _, item := range doc.Outline {
		if item.Start < start || item.Start > end {
			continue
		}
		label := strings.ToLower(item.Label)
		for _, term := range terms {
			if strings.Contains(label, term) {
				score += 5
				break
			}
		}
	}
	return score
}

// sampleSpans 无相关词时均匀抽样，保证覆盖全文
func sampleSpans(chunks []chunkSpan, max int) []chunkSpan {
	if max <= 0 || len(chunks) <= max {
		return chunks
	}
	if max == 1 {
		return []chunkSpan{chunks[0]}
	}
	out := make([]chunkSpan, 0, max)
	step := float64(len(chunks)-1) / float64(max-1)
	for i := 0; i < max; i++ {
		index := int(math.Round(float64(i) * step))
		if index < 0 {
			index = 0
		}
		if index >= len(chunks) {
			index = len(chunks) - 1
		}
		out = append(out, chunks[index])
	}
	return out
}
