package WebLTP

// Web-LTP 报告汇编：逐页情报摘要 + 「月华操作旅程」口吻的搜索报告

import (
	"encoding/json"
	"fmt"
	"strings"
)


// summarizePage 单页情报摘要：DOM 提取文本为主 + 首屏截图为辅（同一次多模态调用互相印证）
func summarizePage(query string, hit SearchHit, page PageExtract, shotDataURL string) (string, error) {
	payload := map[string]any{
		"query_hint": query,
		"url":        page.URL,
		"title":      firstNonEmpty(page.Title, hit.Title),
		"text":       truncateRunes(page.Text, 4500),
		"links":      pageLinksHead(page.Links, 10),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	text := "以下是浏览器元素识别（DOM 提取，非 OCR，文本准确）得到的页面数据：\n" + string(payloadJSON) + "\n\n" +
		"随消息附有该页面首屏截图（若有）。请以 DOM 文本为主要情报来源，并与截图互相印证：" +
		"截图能补充 DOM 读不到的图片/图表信息；若截图显示验证码、错误页、付费墙或与文本明显不符，请在摘要开头如实说明。" +
		"请围绕 query_hint 的检索意图输出 300-900 字的要点式中文摘要（可分点），只输出摘要正文，不要代码块。"
	return chatText([]chatMessage{
		{Role: "system", Content: "你是网页情报整理助手，负责把「DOM 文本 + 页面截图」互相印证后整理成关键情报摘要。"},
		{Role: "user", Content: textWithImage(text, shotDataURL)},
	})
}

// composeReport 汇编搜索报告：优先模型撰写（月华操作旅程口吻），失败时确定性文本兜底
func composeReport(in reportInput) string {
	report, err := composeReportByModel(in)
	if err != nil {
		LoggerGeneralWarn("模型报告撰写失败，使用确定性报告: %v", err)
		return composeReportFallback(in)
	}
	return report
}

// composeReportByModel 模型撰写报告
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
			"order":    i + 1,
			"title":    p.Title,
			"url":      p.URL,
			"domain":   p.Domain,
			"summary":  p.Summary,
			"captures": p.Captures,
		}
		if p.Failed != "" {
			item["failed"] = p.Failed
		}
		if p.Cached {
			item["cached"] = true
		}
		pages = append(pages, item)
	}
	shotNote := fmt.Sprintf("共 %d 张过程截图（未启用本地保存）", in.Screenshots)
	if in.SaveDir != "" {
		shotNote = fmt.Sprintf("共 %d 张过程截图（已保存到本地目录 %s）", in.Screenshots, in.SaveDir)
	}

	payload, err := json.Marshal(map[string]any{
		"instruction": in.Instruction,
		"query":       in.Query,
		"engine":      map[string]any{"count": len(in.Engine.Results), "results": engineHits},
		"pages":       pages,
		"screenshots": shotNote,
		"elapsed":     in.Elapsed,
		"interrupted": in.Interrupted,
	})
	if err != nil {
		return "", err
	}

	engineName := firstNonEmpty(in.EngineName, "必应")
	system := fmt.Sprintf("你是搜索报告撰写助手。月华刚刚亲自驾驶浏览器完成了一次网络搜索，逐页情报来自浏览器元素识别（准确可信）。"+
		"请以叙述月华操作旅程的口吻编写搜索报告：『月华在%s搜索了……，结果页主要由……等站点组成；"+
		"随后月华打开了……页面，看到了……』。结构：① 检索概述（检索词与结果页概览）；"+
		"② 逐页情报（每页：来源域名 + 页面标题 + 关键要点，融数字/名称/结论；打开失败的页面如实说明；"+
		"标记 cached 为 true 的页面是复用的本地缓存摘要，请如实表述为「此前已查看过该页面，本次直接调取了笔记」）；"+
		"③ 综合结论（围绕检索意图汇总结论与建议）。用简洁中文，可分点；"+
		"若 interrupted 为 true，开头注明「浏览器窗口在中途被关闭，以下基于已采集信息」。只输出报告正文，不要代码块。", engineName)

	text := string(payload) + "\n\n随消息附有搜索结果页首屏截图（若有）：请据此描述结果页的整体版面观感" +
		"（如顶部卡片、广告位与自然结果的分布），情报结论仍以 DOM 提取数据为准。"

	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: textWithImage(text, in.EngineShot)},
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
			fmt.Fprintf(&b, "%d. 月华打开了「%s」（%s）：\n%s\n", i+1, firstNonEmpty(p.Title, p.Domain), p.Domain, p.Summary)
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
			b.WriteString(" 张（未启用本地保存）\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// pageLinksHead 取前 n 个链接标题（供摘要载荷）
func pageLinksHead(links []SearchHit, n int) []map[string]any {
	out := make([]map[string]any, 0, n)
	for i, l := range links {
		if i >= n {
			break
		}
		out = append(out, map[string]any{"title": l.Title, "domain": domainOf(l.URL)})
	}
	return out
}
