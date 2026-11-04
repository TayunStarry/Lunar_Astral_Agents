package SearchSummarizer

// report.go 摘要的确定性兜底与长文拼接（不发起模型调用）。

import (
	"fmt"
	"strings"
)

// IsNoRelevance 判断摘要是否为「没有相关内容」的否定占位句（供链路层收口复用）。
func IsNoRelevance(text string) bool { return isNoRelevanceDigest(text) }

// isNoRelevanceDigest 判断摘要是否为「没有相关内容」的否定占位句。
func isNoRelevanceDigest(text string) bool {
	compact := strings.Join(strings.Fields(text), "")
	if compact == "" {
		return false
	}
	for _, marker := range []string{
		"未检索到与问题直接相关的信息",
		"未检索到相关信息",
		"没有检索到与问题直接相关的信息",
		"未找到与问题直接相关的信息",
		"没有找到与问题直接相关的信息",
	} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	return false
}

// pageContentDigest 页面正文兜底摘要：按来源顺序摘取每页正文前 800 字。
func pageContentDigest(sources []Source) string {
	var builder strings.Builder
	for index, source := range sources {
		if index >= 3 {
			break
		}
		body := strings.TrimSpace(source.Text)
		if body == "" {
			continue
		}
		fmt.Fprintf(&builder, "【%s】\n%s\n\n",
			firstNonEmpty(source.Title, source.URL), hardCutRunes(body, pageDigestMaxRunes))
	}
	out := strings.TrimSpace(builder.String())
	if out == "" {
		return "页面正文为空，未获取到可摘要的内容。"
	}
	return out
}

// encodeSources 把全部来源页正文拼接为一个带来源标记的长文
func encodeSources(sources []Source) string {
	var b strings.Builder
	for _, s := range sources {
		fmt.Fprintf(&b, "◆ %s｜%s\n%s\n\n", s.Label, firstNonEmpty(s.Title, s.URL), s.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// firstNonEmpty 返回第一个非空白字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// hardCutRunes 硬切断文本：总长不超过 max 字符（超长时以省略号收尾并标注）
func hardCutRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	return string(r[:max-1]) + "…（已截断）"
}
