package WebLTP

// Web-LTP 页面摘要
//
// 默认路径：提取正文 → 按段落分块 → 逐段摘要 → 合并成单页情报。
// 视觉路径：只有正文少于 VisionMinChars 时才把滚动截图交给多模态模型，
// 用来补充图表、信息卡片等 DOM 文本读不到的内容；文本充足时绝不调用视觉。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// summarizePage 汇总单个结果页情报。
// 返回：摘要、是否使用了视觉、参与摘要的文本段数、错误。
func summarizePage(query string, hit SearchHit, page PageExtract, frames []shotFrame, cfg Config) (string, bool, int, error) {
	// 正文过少：优先让视觉模型看截图；失败时仍可回退到少量文本
	if page.TextChars < cfg.VisionMinChars {
		if len(frames) > 0 {
			summary, verr := summarizeByVision(query, hit, page, frames)
			if verr == nil {
				return summary, true, 0, nil
			}
			if strings.TrimSpace(page.Text) == "" {
				return "", true, 0, fmt.Errorf("视觉摘要失败: %w", verr)
			}
		} else if strings.TrimSpace(page.Text) == "" {
			return "", false, 0, fmt.Errorf("页面正文为空且无可用截图")
		}
	}
	summary, chunks, err := summarizeByText(query, hit, page, cfg)
	return summary, false, chunks, err
}

// summarizeByText 文本主路径：分块摘要后拼接；多段时再做一次合并摘要
func summarizeByText(query string, hit SearchHit, page PageExtract, cfg Config) (string, int, error) {
	text := strings.TrimSpace(page.Text)
	if text == "" {
		return "", 0, fmt.Errorf("页面无可提取文本")
	}
	chunks := splitTextChunks(text, cfg.TextChunkChars, cfg.TextMaxChunks)
	if len(chunks) == 0 {
		return "", 0, fmt.Errorf("页面正文分块为空")
	}
	if len(chunks) == 1 {
		summary, err := summarizeChunk(query, hit, chunks[0], 1, 1)
		return summary, 1, err
	}

	parts := make([]string, 0, len(chunks))
	var firstErr error
	for i, chunk := range chunks {
		part, err := summarizeChunk(query, hit, chunk, i+1, len(chunks))
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			// 单段失败不拖垮整页：保留该段正文前段作为兜底
			parts = append(parts, hardCutRunes(chunk, 600))
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		if firstErr != nil {
			return "", len(chunks), firstErr
		}
		return "", len(chunks), fmt.Errorf("所有文本段摘要均失败")
	}

	merged, err := mergeChunkSummaries(query, hit, parts)
	if err != nil {
		// 合并失败就按段拼接，保证情报不丢
		return strings.Join(parts, "\n"), len(chunks), nil
	}
	return merged, len(chunks), nil
}

// splitTextChunks 按段落装箱为若干文本段（单段超长时按字符硬切）
func splitTextChunks(text string, chunkChars, maxChunks int) []string {
	if chunkChars <= 0 {
		chunkChars = 5000
	}
	if maxChunks <= 0 {
		maxChunks = 4
	}
	paragraphs := strings.Split(text, "\n")
	chunks := make([]string, 0, maxChunks)
	current := &strings.Builder{}
	truncated := false
	flush := func() {
		if strings.TrimSpace(current.String()) == "" {
			return
		}
		chunks = append(chunks, strings.TrimSpace(current.String()))
		current.Reset()
	}
	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		for len([]rune(para)) > chunkChars {
			// 超长单段：先补满当前块，再按字符切片
			room := chunkChars - len([]rune(current.String()))
			if room <= 0 {
				flush()
				room = chunkChars
			}
			runes := []rune(para)
			current.WriteString(string(runes[:room]))
			current.WriteString("\n")
			flush()
			para = string(runes[room:])
		}
		if len([]rune(current.String()))+len([]rune(para))+1 > chunkChars {
			flush()
		}
		current.WriteString(para)
		current.WriteString("\n")
		if len(chunks) >= maxChunks {
			truncated = true
			break
		}
	}
	flush()
	if len(chunks) > maxChunks {
		chunks = chunks[:maxChunks]
		truncated = true
	}
	if truncated && len(chunks) > 0 {
		chunks[len(chunks)-1] += "\n（后续正文因长度限制未纳入摘要）"
	}
	return chunks
}

// summarizeChunk 单段文本摘要
func summarizeChunk(query string, hit SearchHit, chunk string, index, total int) (string, error) {
	system, userTemplate := loadPromptPair("webSearchChunkSummary.md")
	user := renderPrompt(userTemplate,
		"{query}", query,
		"{title}", firstNonEmpty(hit.Title, "（无标题）"),
		"{url}", hit.URL,
		"{index}", fmt.Sprintf("%d", index),
		"{total}", fmt.Sprintf("%d", total),
		"{chunk}", chunk)
	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// mergeChunkSummaries 把同一页面的多段要点合并为单页摘要
func mergeChunkSummaries(query string, hit SearchHit, parts []string) (string, error) {
	var b strings.Builder
	for i, part := range parts {
		fmt.Fprintf(&b, "【第%d段要点】\n%s\n\n", i+1, part)
	}
	system, userTemplate := loadPromptPair("webSearchMergeSummary.md")
	user := renderPrompt(userTemplate,
		"{query}", query,
		"{title}", firstNonEmpty(hit.Title, "（无标题）"),
		"{url}", hit.URL,
		"{parts}", b.String())
	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}

// summarizeByVision 低文本页面：用滚动截图补充 DOM 读不到的视觉信息
func summarizeByVision(query string, hit SearchHit, page PageExtract, frames []shotFrame) (string, error) {
	dataURLs := frameDataURLs(frames, 3)
	if len(dataURLs) == 0 {
		return "", fmt.Errorf("无可用截图")
	}
	payload, err := json.Marshal(map[string]any{
		"query_hint": query,
		"url":        page.URL,
		"title":      firstNonEmpty(page.Title, hit.Title),
		"text":       truncateRunes(page.Text, 1500),
		"text_chars": page.TextChars,
		"images":     page.Images,
		"visuals":    page.Visuals,
	})
	if err != nil {
		return "", err
	}
	system, userTemplate := loadPromptPair("webSearchVisionSummary.md")
	user := renderPrompt(userTemplate, "{payload}", string(payload))
	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: textWithImages(user, dataURLs)},
	})
}

// frameDataURLs 取最多 max 帧转 dataURL（首、中、尾均匀取样，避免只看顶部）
func frameDataURLs(frames []shotFrame, max int) []string {
	picked := pickFrames(frames, max)
	out := make([]string, 0, len(picked))
	for _, f := range picked {
		if len(f.JPEG) == 0 {
			continue
		}
		out = append(out, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(f.JPEG))
	}
	return out
}

// pickFrames 从滚动截图中均匀取样（首帧 + 中间 + 末帧）
func pickFrames(frames []shotFrame, max int) []shotFrame {
	if max <= 0 || len(frames) <= max {
		return frames
	}
	if max == 1 {
		return []shotFrame{frames[0]}
	}
	picked := make([]shotFrame, 0, max)
	for i := 0; i < max; i++ {
		idx := i * (len(frames) - 1) / (max - 1)
		picked = append(picked, frames[idx])
	}
	return picked
}

// summarizeSearchPage 回访搜索页后的纯文本总结（不滚动、不截图）
func summarizeSearchPage(query string, page PageExtract, results []SearchHit) (string, error) {
	hits := make([]map[string]any, 0, len(results))
	for i, hit := range results {
		if i >= 15 {
			break
		}
		hits = append(hits, map[string]any{
			"title":   hit.Title,
			"domain":  domainOf(hit.URL),
			"snippet": truncateRunes(hit.Snippet, 120),
		})
	}
	payload, err := json.Marshal(map[string]any{
		"query":      query,
		"page_title": page.Title,
		"page_text":  truncateRunes(page.Text, 4000),
		"results":    hits,
	})
	if err != nil {
		return "", err
	}
	system, userTemplate := loadPromptPair("webSearchSearchPageSummary.md")
	user := renderPrompt(userTemplate, "{payload}", string(payload))
	return chatText([]chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	})
}
