package WebSearch

// summary.go 统一摘要提取：把首页正文与全部详情页正文拼接为一个长文，
// 连同精炼提示词一口气交给模型一次调用，要求其按月华的原始指令对长文做
// 选择性、针对性的摘要与提取（只保留与原始问题相关的内容，剔除噪音）。
//
// 一次网络搜索只调用两次模型：检索词提炼一次 + 本摘要提取一次；
// 本函数的输出就是递交给对话者的最终内容，不再有报告撰写等后续模型调用。

import (
	"fmt"
	"strings"
)

// composeDigest 统一摘要入口：拼接全部来源页正文为长文，一次模型调用完成提取。
// instruction 是月华的原始指令，作为噪音剔除的准绳。
func composeDigest(instruction, query string, sources []digestSource, profile summaryProfile) (string, error) {
	if len(sources) == 0 {
		return "", fmt.Errorf("无可摘要的页面正文")
	}

	system, userTemplate := loadPromptPair("webSearchDigest.md")
	user := renderPrompt(userTemplate,
		"{instruction}", instruction,
		"{query}", query,
		"{longtext}", encodeSources(sources))
	return chatWithOptions([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}, profileChatOptions(profile))
}

// encodeSources 把全部来源页正文拼接为一个带来源标记的长文
func encodeSources(sources []digestSource) string {
	var b strings.Builder
	for _, s := range sources {
		fmt.Fprintf(&b, "◆ %s｜%s\n%s\n\n", s.Label, firstNonEmpty(s.Title, s.URL), s.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}
