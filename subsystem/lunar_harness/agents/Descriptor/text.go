package Descriptor

// text.go 文本句柄探查：接收自然语言需求，用查询工具分片读取文件后作答
//
// 对话者不需要先知道文件内容：这里只把「规模 + 摘要 + 结构/引用预览」交给模型，
// 正文超出阈值时一律用 read_lines 分片获取，避免一次性撑爆上下文。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// InspectText 按文本句柄执行一次内容探查
//
// @param handle  文本资源句柄（@D.xxx / @T.xxx）
// @param request 对话者的自然语言需求
// @return 描述者给出的答复；句柄失效时返回错误
func InspectText(handle, request string) (string, error) {
	runMu.Lock()
	defer runMu.Unlock()

	request = strings.TrimSpace(request)
	if request == "" {
		request = "请概括这个文件讲了什么，并指出关键结构。"
	}
	res, err := persistence.ResourceResolve(handle)
	if err != nil {
		return "", err
	}
	if res.State != persistence.StateActive {
		return "", fmt.Errorf("%s 的本体已丢失", handle)
	}
	if res.Kind != persistence.KindDocument && res.Kind != persistence.KindTable {
		return "", fmt.Errorf("%s 不是文本类资源", handle)
	}
	doc, err := loadDoc(res)
	if err != nil {
		return "", err
	}

	system := promptText("descriptorFileRole.md")
	briefing := buildBriefing(doc, handle, request)
	messages := []chatMessage{
		{Role: "system", Content: system},
		{Role: "user", Content: briefing},
	}
	tools := fileToolDefs()

	last := ""
	for round := 0; round < maxToolRounds; round++ {
		message, chatErr := chatStream(messages, tools)
		if chatErr != nil && round == 0 {
			// 模型服务不支持工具调用时退化为单轮回答：至少把预览信息用起来
			logger.Warn("CrystalAstral", "描述者：文件探查首轮失败，退化为无工具回答: %v", chatErr)
			fallback, fallbackErr := chatStream(messages, nil)
			if fallbackErr != nil {
				return "", fallbackErr
			}
			if text := strings.TrimSpace(stringOf(fallback.Content)); text != "" {
				return text, nil
			}
			return "", chatErr
		}
		if chatErr != nil {
			return "", chatErr
		}

		text := strings.TrimSpace(stringOf(message.Content))
		if text != "" {
			last = text
		}
		// 回填 assistant 消息时统一成字符串 content，避免部分服务拒绝 null
		message.Content = text
		if len(message.ToolCalls) == 0 {
			if last == "" {
				return "", fmt.Errorf("描述者没有返回内容")
			}
			return last, nil
		}

		// assistant 的 tool_calls 必须原样回填，再逐条给出配对的结果
		messages = append(messages, message)
		for _, call := range message.ToolCalls {
			messages = append(messages, chatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    executeFileTool(doc, call),
			})
		}
	}
	if last != "" {
		return last, nil
	}
	return "", fmt.Errorf("描述者探查轮次超限")
}

// loadDoc 载入文件正文并建立结构/引用索引
func loadDoc(res persistence.ArchiveResource) (*docContext, error) {
	abs := filepath.Join(filepath.Clean(*config.LocalDir), filepath.FromSlash(res.BlobPath))
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	truncated := false
	if runes := []rune(text); len(runes) > maxDocRunes {
		text = string(runes[:maxDocRunes])
		truncated = true
	}
	doc := &docContext{
		Resource:  res,
		Handle:    res.Handle,
		AbsPath:   abs,
		Text:      text,
		Lines:     strings.Split(text, "\n"),
		Truncated: truncated,
	}
	doc.Outline = buildOutline(doc)
	doc.References = buildReferences(doc)
	return doc, nil
}

// buildBriefing 组装文件探查的初始消息
//
// rawHandle 为用户传入的原始句柄：带锚点（|L.. / |§n）时正文只提供该范围，
// 并显式告知模型本次阅读范围，避免整份文件稀释重点。
func buildBriefing(doc *docContext, rawHandle, request string) string {
	summary := strings.TrimSpace(doc.Resource.Summary)
	if summary == "" {
		summary = "（暂无现成摘要）"
	}
	truncated := ""
	if doc.Truncated {
		truncated = "（超大文件已按上限截断）"
	}
	scope, contentBlock := scopedContent(doc, rawHandle)
	return renderPrompt(promptText("descriptorFileBriefing.md"),
		"{request}", request,
		"{scope}", scope,
		"{handle}", doc.Handle,
		"{name}", doc.Resource.Name,
		"{kind}", resourceKindLabel(doc.Resource.Kind),
		"{ext}", strings.TrimPrefix(strings.ToLower(doc.Resource.Ext), "."),
		"{lines}", fmt.Sprintf("%d", len(doc.Lines)),
		"{chars}", fmt.Sprintf("%d", len([]rune(doc.Text))),
		"{truncated}", truncated,
		"{summary}", summary,
		"{outline}", formatOutline(doc.Outline, 20),
		"{references}", formatReferences(doc.References, 15),
		"{content_block}", contentBlock,
	)
}

// scopedContent 生成「阅读范围 + 正文块」。
//
// 无锚点：整份文件（小文件内联，大文件只给开头并引导分片读取）。
// 带锚点：只取该行区间 / 该章节的正文，并让模型知道本次只看这一段；
// 锚点解析失败或取不到内容时回退整份文件，不阻断探查。
func scopedContent(doc *docContext, rawHandle string) (scope, block string) {
	parsed, ok := persistence.ParseHandle(rawHandle)
	if !ok || (!parsed.HasLine && !parsed.HasSection) {
		return "整份文件", buildContentBlock(doc)
	}
	scoped, err := persistence.ResourceRead(rawHandle, persistence.MaxReadBytes)
	if err != nil || strings.TrimSpace(scoped.Content) == "" {
		logger.Warn("CrystalAstral", "描述者：指定范围取回失败，回退整份文件: %v", err)
		return "整份文件（指定范围未能取回，已回退整份）", buildContentBlock(doc)
	}
	if parsed.HasSection {
		scope = fmt.Sprintf("第 %d 节《%s》（第 %d-%d 行，全文 %d 行）",
			parsed.SectionOrdinal, scoped.SectionTitle, scoped.LineStart, scoped.LineEnd, len(doc.Lines))
	} else {
		scope = fmt.Sprintf("第 %d-%d 行（全文 %d 行）", scoped.LineStart, scoped.LineEnd, len(doc.Lines))
	}
	body := scoped.Content
	if len([]rune(body)) > inlineContentLimit {
		body = hardCutRunes(body, inlineContentLimit)
	}
	return scope, "【正文（仅指定范围）】\n" + body
}

// buildContentBlock 小文件内联正文，大文件只给开头并引导分片读取
func buildContentBlock(doc *docContext) string {
	if len([]rune(doc.Text)) <= inlineContentLimit {
		return "【正文】\n" + doc.Text
	}
	return "【正文】\n文件较大，未整体注入。请先用 file_outline 定位行区间，再用 read_lines 分片读取。\n开头片段如下：\n" +
		hardCutRunes(doc.Text, 1200)
}

// resourceKindLabel 类型码中文名
func resourceKindLabel(kind string) string {
	switch kind {
	case persistence.KindDocument:
		return "文档"
	case persistence.KindTable:
		return "表格"
	case persistence.KindImage:
		return "图片"
	case persistence.KindVideo:
		return "视频"
	case persistence.KindAudio:
		return "音频"
	}
	return kind
}
