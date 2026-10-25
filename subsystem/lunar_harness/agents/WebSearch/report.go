package WebSearch

// report.go 报告汇编：搜索页正文 + 逐页文本情报 + 「网络搜索」口吻的结构化报告

import (
	"encoding/json"
	"fmt"
	"strings"
)

// composeReport 汇编搜索报告：优先模型撰写（网络搜索操作旅程口吻），失败时确定性文本兜底
func composeReport(in reportInput, profile summaryProfile) string {
	report, err := composeReportByModel(in, profile)
	if err != nil {
		loggerWarn("模型报告撰写失败，使用确定性报告: %v", err)
		return composeReportFallback(in)
	}
	return report
}

// composeReportByModel 模型撰写报告（纯文本）；模型由摘要档位决定
func composeReportByModel(in reportInput, profile summaryProfile) (string, error) {
	engineHits := make([]map[string]any, 0, len(in.Engine.Results))
	for i, hit := range in.Engine.Results {
		if i >= 15 {
			break
		}
		engineHits = append(engineHits, map[string]any{
			"title":   hit.Title,
			"domain":  domainOf(hit.URL),
			"snippet": truncateRunes(hit.Snippet, 100),
		})
	}
	pages := make([]map[string]any, 0, len(in.Pages))
	for i, p := range in.Pages {
		pages = append(pages, map[string]any{
			"order":      i + 1,
			"title":      p.Title,
			"url":        p.URL,
			"domain":     p.Domain,
			"summary":    p.Summary,
			"text_chars": p.TextChars,
			"chunks":     p.Chunks,
		})
	}

	payload, err := json.Marshal(map[string]any{
		"instruction":      in.Instruction,
		"query":            in.Query,
		"engine":           map[string]any{"count": len(in.Engine.Results), "results": engineHits},
		"search_page_text": truncateRunes(in.SearchPageText, 4000),
		"pages":            pages,
		"elapsed":          in.Elapsed,
		"interrupted":      in.Interrupted,
	})
	if err != nil {
		return "", err
	}

	engineName := firstNonEmpty(in.EngineName, "必应")
	system := renderPrompt(loadPrompt("webSearchReport.md"), "{engine}", engineName)

	return chatWithOptions([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payload)},
	}, profileChatOptions(profile))
}

// composeReportFallback 确定性报告兜底（模型不可用时）
func composeReportFallback(in reportInput) string {
	engineName := firstNonEmpty(in.EngineName, "必应")
	var b strings.Builder
	b.WriteString("【网络搜索报告】")
	if in.Interrupted {
		b.WriteString("（浏览器窗口在中途被关闭，以下基于已采集信息）")
	}
	b.WriteString("\n\n检索词「")
	b.WriteString(in.Query)
	b.WriteString("」，")
	b.WriteString(engineName)
	b.WriteString("结果页共识别 ")
	fmt.Fprint(&b, len(in.Engine.Results))
	b.WriteString(" 条结果，耗时 ")
	b.WriteString(in.Elapsed)
	b.WriteString("。\n")

	if strings.TrimSpace(in.SearchPageText) != "" {
		b.WriteString("\n◆ 搜索页概览：\n")
		b.WriteString(truncateRunes(in.SearchPageText, 1500))
		b.WriteString("\n")
	}

	if len(in.Engine.Results) > 0 {
		b.WriteString("\n◆ 结果页概览：\n")
		for i, hit := range in.Engine.Results {
			if i >= 8 {
				break
			}
			fmt.Fprintf(&b, "%d. %s（%s）\n", i+1, hit.Title, domainOf(hit.URL))
		}
	}

	if len(in.Pages) > 0 {
		b.WriteString("\n◆ 逐页情报：\n")
		for i, p := range in.Pages {
			fmt.Fprintf(&b, "%d. 打开了「%s」（%s，正文 %d 字）：\n%s\n", i+1, firstNonEmpty(p.Title, p.Domain), p.Domain, p.TextChars, p.Summary)
		}
	} else {
		b.WriteString("\n◆ 逐页情报：本次未获取到可用的详情页内容。\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
