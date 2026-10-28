package WebSearch

// report.go 输出收口：摘要提取的产出（digest）就是递交给对话者的最终内容，
// 不再有报告撰写模型调用——一次网络搜索只调用两次模型（检索词提炼 + 摘要提取）。
// 仅当摘要提取失败时，用确定性文本兜底，保证调用方拿到结构清晰的结果。

import (
	"fmt"
	"strings"
)

// composeReport 输出收口：优先采用模型的摘要提取结果；失败时确定性兜底
func composeReport(in reportInput) string {
	digest := strings.TrimSpace(in.Digest)
	if digest != "" && !isNoRelevanceDigest(digest) {
		return digest
	}
	if digest != "" {
		loggerWarn("摘要提取为「未检索到」占位句，改用页面内容兜底")
	} else {
		loggerWarn("摘要提取为空，使用确定性兜底输出")
	}
	return composeReportFallback(in)
}

// composeReportFallback 确定性兜底（摘要模型调用失败时）
func composeReportFallback(in reportInput) string {
	engineName := firstNonEmpty(in.EngineName, "必应")
	var b strings.Builder
	b.WriteString("【网络搜索兜底输出】")
	if in.Interrupted {
		b.WriteString("（浏览器窗口在中途被关闭，以下基于已采集信息）")
	}
	b.WriteString("\n\n本次未能完成针对性摘要提取，以下为原始采集信息。\n检索词「")
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
		b.WriteString(hardCutRunes(in.SearchPageText, 1500))
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
		b.WriteString("\n◆ 打开的页面：\n")
		for i, p := range in.Pages {
			fmt.Fprintf(&b, "%d. 「%s」（%s，正文 %d 字）\n", i+1, firstNonEmpty(p.Title, p.Domain), p.Domain, p.TextChars)
		}
		b.WriteString("\n◆ 页面内容摘要：\n")
		for i, p := range in.Pages {
			if i >= 3 {
				break
			}
			text := strings.TrimSpace(p.Text)
			if text == "" {
				continue
			}
			fmt.Fprintf(&b, "【%s】\n%s\n\n", firstNonEmpty(p.Title, p.Domain), hardCutRunes(text, 800))
		}
	} else {
		b.WriteString("\n◆ 打开的页面：本次未获取到可用的详情页内容。\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
