package WebLTP

// Web-LTP 报告汇编：搜索页总结 + 逐页文本情报 + 「网络搜索」口吻的报告

import (
	"encoding/json"
	"fmt"
	"strings"
)

// composeReport 汇编搜索报告：优先模型撰写（网络搜索操作旅程口吻），失败时确定性文本兜底
func composeReport(in reportInput) string {
	report, err := composeReportByModel(in)
	if err != nil {
		LoggerGeneralWarn("模型报告撰写失败，使用确定性报告: %v", err)
		return composeReportFallback(in)
	}
	return report
}

// composeReportByModel 模型撰写报告（纯文本，不带截图）
func composeReportByModel(in reportInput) (string, error) {
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
		item := map[string]any{
			"order":      i + 1,
			"title":      p.Title,
			"url":        p.URL,
			"domain":     p.Domain,
			"summary":    p.Summary,
			"text_chars": p.TextChars,
			"chunks":     p.Chunks,
			"vision":     p.Vision,
		}
		if p.Failed != "" {
			item["failed"] = p.Failed
		}
		if p.Cached {
			item["cached"] = true
		}
		pages = append(pages, item)
	}
	shotNote := fmt.Sprintf("共 %d 张按需截图（未启用本地保存）", in.Screenshots)
	if in.SaveDir != "" {
		shotNote = fmt.Sprintf("共 %d 张按需截图（已保存到本地目录 %s）", in.Screenshots, in.SaveDir)
	}

	payload, err := json.Marshal(map[string]any{
		"instruction":    in.Instruction,
		"query":          in.Query,
		"engine":         map[string]any{"count": len(in.Engine.Results), "results": engineHits},
		"search_summary": in.SearchSummary,
		"pages":          pages,
		"screenshots":    shotNote,
		"elapsed":        in.Elapsed,
		"interrupted":    in.Interrupted,
	})
	if err != nil {
		return "", err
	}

	engineName := firstNonEmpty(in.EngineName, "必应")
	system := fmt.Sprintf("你是搜索报告撰写助手。月华刚刚用浏览器完成了一次网络搜索：先在%s结果页从上到下完整浏览，"+
		"再逐个进入结果页从上到下阅读正文，最后回访搜索页做纯文本总结。逐页情报来自正文提取与分块摘要（准确可信）。"+
		"请以叙述月华网络搜索过程的口吻编写搜索报告：『月华在%s搜索了……，结果页主要由……等站点组成；"+
		"随后月华打开了……页面，读到了……；最后回到搜索页做了总结……』。结构：① 检索概述（检索词与结果页概览，引用 search_summary）；"+
		"② 逐页情报（每页：来源域名 + 页面标题 + 关键要点，融数字/名称/结论；打开失败的页面如实说明；"+
		"标记 cached 为 true 的页面是复用的本地缓存摘要，请如实表述为「此前已查看过该页面，本次直接调用了笔记」；"+
		"vision 为 true 的页面表示正文很少、月华通过页面截图补充了视觉信息，可如实说明）；"+
		"③ 综合结论（围绕检索意图汇总结论与建议）。用简洁中文，可分点；"+
		"若 interrupted 为 true，开头注明「浏览器窗口在中途被关闭，以下基于已采集信息」。只输出报告正文，不要代码块。", engineName, engineName)

	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: string(payload)},
	})
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

	if in.SearchSummary != "" {
		b.WriteString("\n◆ 搜索页总结：\n")
		b.WriteString(in.SearchSummary)
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
			if p.Failed != "" {
				fmt.Fprintf(&b, "%d. 打开了「%s」（%s）——页面打开失败：%s\n", i+1, p.Title, p.Domain, p.Failed)
				continue
			}
			if p.Cached {
				fmt.Fprintf(&b, "%d. 月华此前已查看过「%s」（%s），本次直接调取缓存摘要：\n%s\n", i+1, firstNonEmpty(p.Title, p.Domain), p.Domain, p.Summary)
				continue
			}
			fmt.Fprintf(&b, "%d. 月华打开了「%s」（%s，正文 %d 字）：\n%s\n", i+1, firstNonEmpty(p.Title, p.Domain), p.Domain, p.TextChars, p.Summary)
		}
	}

	if in.Screenshots > 0 {
		if in.SaveDir != "" {
			b.WriteString("\n◆ 过程截图：")
			fmt.Fprint(&b, in.Screenshots)
			b.WriteString(" 张，已保存到 ")
			b.WriteString(in.SaveDir)
			b.WriteString("\n")
		} else {
			b.WriteString("\n◆ 过程截图：")
			fmt.Fprint(&b, in.Screenshots)
			b.WriteString(" 张（按需截图，未启用本地保存）\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
