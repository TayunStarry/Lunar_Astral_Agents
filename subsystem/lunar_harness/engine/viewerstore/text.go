package viewerstore

// text.go 记忆正文的通用文本处理：机械内容识别、结论截断与旧正文分段。
//
// 这些规则同时被存储层迁移与编排层的记忆清洗使用，因此放在存储层统一维护。

import (
	"strings"

	"LunarSubsystem/LunarHarness/internal/textutil"
)

// LooksMechanical 判断一段文本是不是机械过程/运行统计，而不是人格化记忆。
func LooksMechanical(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range mechanicalMemoryMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// Conclusion 只取解读的首句作为长期结论，其余细节留在正文里。
func Conclusion(text string) string {
	runes := []rune(strings.TrimSpace(text))
	for index, char := range runes {
		if index >= ConclusionMaxRunes {
			break
		}
		switch char {
		case '。', '！', '？', '!', '?', '\n':
			return string(runes[:index+1])
		}
	}
	if len(runes) > ConclusionMaxRunes {
		return string(runes[:ConclusionMaxRunes])
	}
	return string(runes)
}

// memorySegment 取「标记：」后到下一个 ｜ 之间的内容。
func memorySegment(text, marker string) string {
	index := strings.Index(text, marker)
	if index < 0 {
		return ""
	}
	rest := text[index+len(marker):]
	if end := strings.Index(rest, "｜"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

// memoryLastSegment 取正文最后一个 ｜ 之后的结论。
func memoryLastSegment(text string) string {
	parts := strings.Split(text, "｜")
	return strings.TrimSpace(parts[len(parts)-1])
}

// stripMemoryMarker 去掉旧正文里的展示标记（兼容全角/半角冒号）。
func stripMemoryMarker(text, marker string) string {
	text = strings.TrimSpace(text)
	for _, candidate := range []string{marker, strings.TrimSuffix(marker, "：") + ":"} {
		if strings.HasPrefix(text, candidate) {
			return strings.TrimSpace(text[len(candidate):])
		}
	}
	return text
}

// ==== BV 号规则 ====
// 唯一实现在 internal/textutil/bvid.go：它是无依赖的叶子包，
// 可同时被 engine/resource、本包与 chain/viewer 引用；依赖方向始终是 chain → engine，
// 不会出现 engine 包之间的横向依赖。这里只保留包内转调入口。

// ExtractBVID 从任意文本里提取第一个 BV 号；找不到返回空串。
func ExtractBVID(text string) string { return textutil.ExtractBVID(text) }

// IsBVID 判断整串是否 BV 号（用于摘要库主键判定）。
func IsBVID(key string) bool { return textutil.IsBVID(key) }

// ReplaceBVID 把文本中所有 BV 号交给 repl 转换（批量登记资源句柄用）。
func ReplaceBVID(text string, repl func(string) string) string {
	return textutil.ReplaceBVID(text, repl)
}
