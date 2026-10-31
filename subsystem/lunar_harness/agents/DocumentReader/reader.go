package DocumentReader

// reader.go 文献解析入口：接收自然语言需求，用查询工具分片读取文件后作答
//
// 对话者不需要先知道文件内容：这里只把「格式画像 + 规模 + 摘要 + 结构/引用预览」
// 交给模型；正文超出 10000 字时一律走分批工具，避免一次性撑爆上下文与内存。

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/agentchat"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// InspectText 按文本句柄执行一次内容探查
//
// @param handle  文本资源句柄（@D.xxx；表格与源码同样使用 @D）
// @param request 对话者的自然语言需求
// @return 文献解析给出的答复；句柄失效或非文本资源时返回错误
func InspectText(handle, request string) (string, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		request = "请概括这个文件讲了什么，并指出关键结构。"
	}
	res, err := resource.Resolve(handle)
	if err != nil {
		return "", err
	}
	if res.State != resource.StateActive {
		return "", fmt.Errorf("%s 的本体已丢失", handle)
	}
	if res.Kind != resource.KindDocument && res.Kind != resource.KindTable {
		return "", fmt.Errorf("%s 不是文本类资源", handle)
	}
	doc, err := loadDoc(res)
	if err != nil {
		return "", err
	}
	// 能力边界：二进制容器与二进制文档不按文本扫描，交给归档解析智能体
	if doc.Profile.Binary {
		return "", fmt.Errorf("%s 是%s（二进制，%s），请交给归档解析智能体处理", handle, doc.Profile.Label, emptyExt(doc.Profile.Ext))
	}

	// 大文件优先走轻量扫描通道：主模型规划、轻量模型逐片摘要、主模型归并
	if answer, handled, scanErr := scanAnswer(doc, handle, request); handled {
		if scanErr != nil {
			logger.Warn("CrystalAstral", "文献解析：轻量扫描失败，回退直读流程: %v", scanErr)
		} else if strings.TrimSpace(answer) != "" {
			return answer, nil
		}
	}

	system := agentchat.PromptText("documentFileRole.md")
	briefing := buildBriefing(doc, handle, request)
	messages := []agentchat.Message{
		{Role: "system", Content: system},
		{Role: "user", Content: briefing},
	}
	return runToolLoop(doc, messages, fileToolDefs(), maxToolRounds)
}

// ProfileOf 返回资源的内容格式画像（供句柄系统与集成测试直接查询）
func ProfileOf(handle string) (string, error) {
	res, err := resource.Resolve(handle)
	if err != nil {
		return "", err
	}
	if res.State != resource.StateActive {
		return "", fmt.Errorf("%s 的本体已丢失", handle)
	}
	if res.Kind != resource.KindDocument && res.Kind != resource.KindTable {
		return "", fmt.Errorf("%s 不是文本类资源", handle)
	}
	doc, err := loadDoc(res)
	if err != nil {
		return "", err
	}
	text := "【格式识别】" + formatProfileText(doc.Profile)
	if doc.Truncated {
		text += "\n（超大文件已按上限截断保护）"
	}
	if len(doc.Chunks) > 0 {
		text += fmt.Sprintf("\n【分批计划】共 %d 片，单片 ≤ %d 字", len(doc.Chunks), chunkRunes)
	}
	return text, nil
}

// loadDoc 载入文件正文并建立格式画像、结构/引用索引与分批计划
func loadDoc(res resource.Resource) (*docContext, error) {
	abs := filepath.Join(filepath.Clean(config.LocalDir), filepath.FromSlash(res.BlobPath))
	info, statErr := os.Stat(abs)
	if statErr != nil {
		return nil, fmt.Errorf("读取文件失败: %w", statErr)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s 是目录，不是文件", res.Handle)
	}
	if info.Size() > maxDocBytes {
		return nil, fmt.Errorf("文件过大（%d 字节），超过单文件解析上限 %d 字节", info.Size(), maxDocBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}
	text := stripBOM(string(data))
	doc := &docContext{
		Resource: res,
		Handle:   res.Handle,
		AbsPath:  abs,
		Text:     text,
		Profile:  detectProfile(res, info.Size(), text),
	}
	if doc.Profile.Binary {
		return doc, nil
	}
	if utf8.RuneCountInString(text) > maxDocRunes {
		runes := []rune(text)
		text = string(runes[:maxDocRunes])
		doc.Truncated = true
	}
	doc.Text = text
	doc.Lines = strings.Split(text, "\n")
	doc.Profile.Runes = utf8.RuneCountInString(text)
	doc.Profile.Lines = len(doc.Lines)
	doc.Profile.SizeLabel, doc.Profile.Strategy, doc.Profile.Large = sizeHint(doc.Profile.Runes)
	doc.Outline = buildOutline(doc)
	doc.References = buildReferences(doc)
	doc.Chunks = buildChunkPlan(doc.Lines)
	return doc, nil
}

// buildChunkPlan 按默认字符上限切分正文，保证每一片都可独立读取
func buildChunkPlan(lines []string) []chunkSpan {
	return buildChunkPlanWith(lines, chunkRunes)
}

// buildChunkPlanWith 按指定字符上限切分正文（轻量扫描可按模型上下文重建分批）
func buildChunkPlanWith(lines []string, limit int) []chunkSpan {
	if len(lines) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = chunkRunes
	}
	spans := make([]chunkSpan, 0, len(lines)/200+1)
	start := 0
	chars := 0
	for index, line := range lines {
		lineChars := utf8.RuneCountInString(line) + 1
		if chars > 0 && chars+lineChars > limit {
			spans = append(spans, chunkSpan{Index: len(spans), StartLine: start + 1, EndLine: index, Chars: chars})
			start = index
			chars = 0
		}
		chars += lineChars
	}
	if start < len(lines) {
		spans = append(spans, chunkSpan{Index: len(spans), StartLine: start + 1, EndLine: len(lines), Chars: chars})
	}
	return spans
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
	return agentchat.RenderPrompt(agentchat.PromptText("documentFileBriefing.md"),
		"{request}", request,
		"{scope}", scope,
		"{handle}", doc.Handle,
		"{name}", doc.Resource.Name,
		"{kind}", resourceKindLabel(doc.Resource.Kind),
		"{ext}", strings.TrimPrefix(strings.ToLower(doc.Resource.Ext), "."),
		"{profile}", formatProfileText(doc.Profile),
		"{lines}", fmt.Sprintf("%d", len(doc.Lines)),
		"{chars}", fmt.Sprintf("%d", doc.Profile.Runes),
		"{truncated}", truncated,
		"{chunks}", fmt.Sprintf("%d", len(doc.Chunks)),
		"{summary}", summary,
		"{outline}", formatOutline(doc.Outline, 20),
		"{references}", formatReferences(doc.References, 15),
		"{content_block}", contentBlock,
	)
}

// scopedContent 生成「阅读范围 + 正文块」。
//
// 无锚点：整份文件（小文件内联，大文件只给开头并引导分批读取）。
// 带锚点：只取该行区间 / 该章节的正文，并让模型知道本次只看这一段；
// 锚点解析失败或取不到内容时回退整份文件，不阻断探查。
func scopedContent(doc *docContext, rawHandle string) (scope, block string) {
	parsed, ok := resource.ParseHandle(rawHandle)
	if !ok || (!parsed.HasLine && !parsed.HasSection) {
		return "整份文件", buildContentBlock(doc)
	}
	scoped, err := resource.Read(rawHandle, resource.MaxReadBytes)
	if err != nil || strings.TrimSpace(scoped.Content) == "" {
		logger.Warn("CrystalAstral", "文献解析：指定范围取回失败，回退整份文件: %v", err)
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
		body = agentchat.HardCutRunes(body, inlineContentLimit)
	}
	return scope, "【正文（仅指定范围）】\n" + body
}

// buildContentBlock 小文件内联正文，大文件只给开头并引导分批读取
func buildContentBlock(doc *docContext) string {
	if doc.Profile.Runes <= inlineContentLimit {
		return "【正文】\n" + doc.Text
	}
	return fmt.Sprintf("【正文】\n文件超过 %d 字，未整体注入。请先用 chunk_plan 取得分批区间，"+
		"再用 read_chunk 或 read_lines 逐片读取（单次 ≤ %d 字）。\n开头片段如下：\n%s",
		inlineContentLimit, maxToolReadChars, agentchat.HardCutRunes(doc.Text, 1200))
}

// resourceKindLabel 类型码中文名
func resourceKindLabel(kind string) string {
	return resource.KindLabel(kind)
}

// stripBOM 去掉 UTF-8 BOM
func stripBOM(text string) string {
	return strings.TrimPrefix(text, "\ufeff")
}
