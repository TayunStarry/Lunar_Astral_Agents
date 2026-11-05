package viewerstore

// legacy.go 旧观影记忆的身份识别与正文解析。
//
// 迁移对象：
//   - Source == memorySource 的条目；
//   - 旧版本无 Source、但正文以 [B站视频] 开头的条目（显式跳过对话前缀，避免误删对话记忆）。

import (
	"strings"
	"time"
)

// selectLegacyViewerEntries 从扫描结果里筛出真正的观影历史条目。
//
// 身份只认两种：Source == memorySource，或「无来源 + [B站视频] 前缀 + 不是对话正文」。
func selectLegacyViewerEntries(all []LegacyMemoryEntry) []LegacyMemoryEntry {
	out := make([]LegacyMemoryEntry, 0, len(all))
	seen := make(map[string]bool, len(all))
	for _, entry := range all {
		id := strings.TrimSpace(entry.ID)
		if id == "" || seen[id] || !isLegacyViewerEntry(entry) {
			continue
		}
		seen[id] = true
		out = append(out, entry)
	}
	return out
}

// isLegacyViewerEntry 条目是否属于观影历史数据。
func isLegacyViewerEntry(entry LegacyMemoryEntry) bool {
	source := strings.TrimSpace(entry.Source)
	if source == memorySource {
		return true
	}
	if source != "" {
		return false
	}
	if isDialogueContent(entry.Content) {
		return false
	}
	return hasBilibiliLegacyPrefix(entry.Content)
}

// hasBilibiliLegacyPrefix 正文第一个 [] 段是否为旧观影展示前缀。
func hasBilibiliLegacyPrefix(content string) bool {
	label, ok := firstBracketLabel(content)
	if !ok {
		return false
	}
	for _, prefix := range legacyViewerPrefixes {
		if label == prefix {
			return true
		}
	}
	return false
}

// firstBracketLabel 取正文开头第一个 [段] 的内容。
func firstBracketLabel(content string) (string, bool) {
	text := strings.TrimSpace(content)
	if !strings.HasPrefix(text, "[") {
		return "", false
	}
	end := strings.Index(text, "]")
	if end <= 1 {
		return "", false
	}
	return strings.TrimSpace(text[1:end]), true
}

// isDialogueContent 正文是否为对话链路写入的消息（[用户: X]: / [群聊: Y][用户: Z]: ）。
func isDialogueContent(content string) bool {
	text := strings.TrimSpace(content)
	for strings.HasPrefix(text, "[") {
		end := strings.Index(text, "]")
		if end <= 1 {
			return false
		}
		if isDialogueLabel(strings.TrimSpace(text[1:end])) {
			return true
		}
		text = strings.TrimSpace(text[end+1:])
	}
	return false
}

// isDialogueLabel 判断一个 [段] 是否是对话前缀标签。
func isDialogueLabel(label string) bool {
	for _, marker := range dialoguePrefixLabels {
		if !strings.HasPrefix(label, marker) {
			continue
		}
		rest := strings.TrimSpace(label[len(marker):])
		// 真实格式是「用户: 名字」「群聊: 名字」，标签后面紧跟冒号
		if strings.HasPrefix(rest, ":") || strings.HasPrefix(rest, "：") {
			return true
		}
	}
	return false
}

// parseLegacyMemoryContent 解析旧正文：剥掉 [B站视频] 前缀，再取 [类别] 与正文。
func parseLegacyMemoryContent(content string) (kind, text string, ok bool) {
	text = strings.TrimSpace(content)
	sawViewerPrefix := false
	for strings.HasPrefix(text, "[") {
		end := strings.Index(text, "]")
		if end <= 1 {
			return "", "", false
		}
		label := strings.TrimSpace(text[1:end])
		if isLegacyViewerPrefix(label) {
			sawViewerPrefix = true
			text = strings.TrimSpace(text[end+1:])
			continue
		}
		if memoryKinds[label] {
			return label, strings.TrimSpace(text[end+1:]), true
		}
		return "", "", false
	}
	// 只在明确见过 [B站视频] 前缀时，才把「前缀后直接是正文」兜底为印象。
	if !sawViewerPrefix || text == "" {
		return "", "", false
	}
	return "印象", text, true
}

// isLegacyViewerPrefix 是否是历史 [B站视频] 展示前缀。
func isLegacyViewerPrefix(label string) bool {
	for _, prefix := range legacyViewerPrefixes {
		if label == prefix {
			return true
		}
	}
	return false
}

// parseLegacyObservation 从旧正文（《标题》｜URL｜主题：…｜氛围：…｜结论）还原结构化观感。
func parseLegacyObservation(body string, at int64) Observation {
	bv := ExtractBVID(body)
	if bv == "" {
		return Observation{}
	}
	if at == 0 {
		at = time.Now().Unix()
	}
	obs := Observation{
		BV:         bv,
		Topics:     memorySegment(body, "主题："),
		Tone:       memorySegment(body, "氛围："),
		Conclusion: memoryLastSegment(body),
		UpdatedAt:  at,
	}
	if match := legacyTitleRe.FindStringSubmatch(body); len(match) > 1 {
		obs.Title = strings.TrimSpace(match[1])
	}
	if match := legacyURLRe.FindString(body); match != "" {
		obs.URL = match
	}
	return obs
}
