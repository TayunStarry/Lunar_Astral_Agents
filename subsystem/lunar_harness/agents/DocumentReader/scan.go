package DocumentReader

// scan.go 大文件轻量扫描通道的编排。
//
// 流程：主模型规划 → 卸载主模型 / 加载轻量模型 → 逐片扫描 → 卸载轻量模型 /
// 恢复主模型 → 主模型归并笔记并按需回读原文。
// 只在大文件、片数足够且配置开启时启用；其余情况仍走 reader.go 的直读流程。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
	"LunarSubsystem/LunarHarness/engine/model/llama"
	"LunarSubsystem/LunarHarness/engine/model/modellease"
)

// scanAnswer 尝试轻量扫描通道；handled=false 表示调用方应继续走原直读流程
func scanAnswer(doc *docContext, handle, request string) (string, bool, error) {
	if !shouldScan(doc) {
		return "", false, nil
	}
	plan := planScan(doc, handle, request)
	if plan.Mode == scanModeDirect {
		return "", false, nil
	}
	chunks := scanChunks(doc)
	spans := selectScanSpans(doc, chunks, plan, config.DocumentScanMaxChunks)
	if len(spans) == 0 {
		return "", false, nil
	}

	restore, ok := acquireScanModel()
	if !ok {
		return "", false, nil
	}
	notes := scanSpans(doc, plan, spans)
	restore()
	if len(notes) == 0 {
		return "", false, nil
	}

	answer, err := mergeScanNotes(doc, handle, request, plan, chunks, spans, notes)
	if err != nil || strings.TrimSpace(answer) == "" {
		logger.Warn("CrystalAstral", "文献解析：扫描笔记归并失败，直接回吐笔记: %v", err)
		return notesFallback(notes), true, nil
	}
	return strings.TrimSpace(answer), true, nil
}

// shouldScan 判断本次文件是否值得走轻量扫描通道
func shouldScan(doc *docContext) bool {
	if config.DocumentScanSwap != nil && !*config.DocumentScanSwap {
		return false
	}
	if strings.TrimSpace(config.DocumentScanModel) == "" {
		return false
	}
	if doc.Profile.Binary {
		return false
	}
	if doc.Profile.Runes < config.DocumentScanThreshold {
		return false
	}
	return len(doc.Chunks) >= scanMinChunks
}

// scanChunks 返回扫描用的分批计划；配置的单片上限与默认不同时按配置重建
func scanChunks(doc *docContext) []chunkSpan {
	limit := config.DocumentScanChunkRunes
	if limit <= 0 || limit == chunkRunes {
		return doc.Chunks
	}
	return buildChunkPlanWith(doc.Lines, limit)
}

// acquireScanModel 获取模型切换租约并卸载主模型；返回释放函数
func acquireScanModel() (func(), bool) {
	releaseLease := modellease.AcquireSwap()
	if _, err := llama.UnloadModelByID(config.AgentMultimodalModel); err != nil {
		logger.SubWarn("CrystalAstral", "文献解析", "卸载主模型[%s]失败，继续尝试轻量扫描模型: %v", config.AgentMultimodalModel, err)
	} else {
		logger.Info("CrystalAstral", "文献解析：已卸载主模型[%s]，改用轻量扫描模型[%s]", config.AgentMultimodalModel, config.DocumentScanModel)
	}
	return func() {
		if _, err := llama.UnloadModelByID(config.DocumentScanModel); err != nil {
			logger.SubWarn("CrystalAstral", "文献解析", "收尾卸载轻量模型[%s]失败: %v", config.DocumentScanModel, err)
		} else {
			logger.Info("CrystalAstral", "文献解析：已卸载轻量扫描模型[%s]", config.DocumentScanModel)
		}
		releaseLease()
	}, true
}

// scanSpans 逐片扫描；连续两片失败视为轻量模型不可用，提前结束
func scanSpans(doc *docContext, plan scanPlan, spans []chunkSpan) []scanNote {
	notes := make([]scanNote, 0, len(spans))
	failures := 0
	for _, span := range spans {
		note, err := scanOne(doc, plan, span)
		if err != nil {
			failures++
			logger.Warn("CrystalAstral", "文献解析：片 #%d 扫描失败: %v", span.Index, err)
			if failures >= 2 && len(notes) == 0 {
				break
			}
			continue
		}
		failures = 0
		notes = append(notes, note)
	}
	return notes
}

// scanOne 扫描单个分片；每次请求只带当前片，不累积历史
func scanOne(doc *docContext, plan scanPlan, span chunkSpan) (scanNote, error) {
	text := chunkText(doc, span, config.DocumentScanChunkRunes)
	system := agentchat.PromptText("documentScanRole.md")
	user := agentchat.RenderPrompt(agentchat.PromptText("documentScanChunk.md"),
		"{goal}", plan.Goal,
		"{mode}", plan.Mode,
		"{focus}", strings.Join(plan.Focus, "、"),
		"{name}", doc.Resource.Name,
		"{handle}", doc.Handle,
		"{index}", fmt.Sprintf("%d", span.Index),
		"{start}", fmt.Sprintf("%d", span.StartLine),
		"{end}", fmt.Sprintf("%d", span.EndLine),
		"{total}", fmt.Sprintf("%d", len(doc.Chunks)),
		"{text}", text,
	)
	temperature := 0.2
	message, err := agentchat.ChatStreamWithOptions(scanRoleLabel,
		[]agentchat.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		}, nil, agentchat.ChatOptions{
			Model:       config.DocumentScanModel,
			Temperature: &temperature,
			MaxTokens:   scanNoteMaxTokens,
			HoldLease:   true,
		})
	if err != nil {
		return scanNote{}, err
	}
	out := strings.TrimSpace(agentchat.StringOf(message.Content))
	if out == "" {
		return scanNote{}, fmt.Errorf("片 #%d 扫描结果为空", span.Index)
	}
	return scanNote{Span: span, Text: out}, nil
}

// chunkText 取出一片的正文并按扫描字符上限截断
func chunkText(doc *docContext, span chunkSpan, limit int) string {
	start, end := span.StartLine, span.EndLine
	if start < 1 {
		start = 1
	}
	if end > len(doc.Lines) {
		end = len(doc.Lines)
	}
	if start > end {
		return ""
	}
	text := strings.Join(doc.Lines[start-1:end], "\n")
	if len([]rune(text)) > limit {
		text = agentchat.HardCutRunes(text, limit)
	}
	return text
}

// mergeScanNotes 主模型归并笔记，并允许用查询工具回读原文核对
func mergeScanNotes(doc *docContext, handle, request string, plan scanPlan, chunks, spans []chunkSpan, notes []scanNote) (string, error) {
	system := agentchat.PromptText("documentScanMerge.md")
	user := buildMergeBriefing(doc, handle, request, plan, chunks, spans, notes)
	messages := []agentchat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
	return runToolLoop(doc, messages, fileToolDefs(), scanMergeRounds)
}

// notesFallback 归并失败时的确定性兜底：按行区间回吐扫描笔记
func notesFallback(notes []scanNote) string {
	var builder strings.Builder
	builder.WriteString("（轻量扫描笔记汇总；归并未完成，以下按行区间保留原始笔记）\n")
	chars := 0
	for _, note := range notes {
		block := fmt.Sprintf("\n【片 #%d L%d-%d】\n%s\n",
			note.Span.Index, note.Span.StartLine, note.Span.EndLine,
			truncateRunes(note.Text, scanNoteMaxChars))
		blockChars := len([]rune(block))
		if chars+blockChars > scanFallbackChars {
			break
		}
		builder.WriteString(block)
		chars += blockChars
	}
	return builder.String()
}
