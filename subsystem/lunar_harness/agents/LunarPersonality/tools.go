package LunarPersonality

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/ArchiveReader"
	"LunarSubsystem/LunarHarness/agents/FileEditor"
	"LunarSubsystem/LunarHarness/agents/Vision"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/locator"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// ==== 工具定义 ====

const (
	// techniqueToolName 技法创作聚合工具名
	techniqueToolName = "dispatch_technique"
	// techniquePainter 绘图子工具
	techniquePainter = "painter"
	// techniqueMusician 音乐子工具
	techniqueMusician = "musician"
	// techniqueScreenshoter 截图子工具
	techniqueScreenshoter = "screenshoter"
	// techniqueFileEditor 文件编辑子工具
	techniqueFileEditor = "file_editor"
	// fileEditorTimeout 单次文件编辑任务的超时上限
	fileEditorTimeout = 15 * time.Minute
)

var agentControlTools = []ToolCall{
	functionTool(techniqueToolName, "使用自然语言去调度专用子智能体进行技法创作",
		map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{techniquePainter, techniqueMusician, techniqueScreenshoter, techniqueFileEditor},
				"description": "painter=绘画创作、musician=音乐创作、screenshoter=屏幕截图、file_editor=文件编辑",
			},
			"instruction": map[string]any{
				"type":        "string",
				"description": "例如: 画一张赛博朋克风格的自画像；创作一段安静的钢琴曲；截取当前屏幕上弹出的对话框；把文档库里的笔记整理成一篇新文章",
			},
			"reference_tag": map[string]any{
				"type":        "string",
				"description": "可选，仅 painter 使用：图生图基座图的图片句柄（如 @I.003）",
			},
		}, []string{"type", "instruction"}),
}

var fetchResourceTools = []ToolCall{
	functionTool("fetch_resources", "对文件内容与长期记忆进行检索：type=memory 查记忆、type=handle 按句柄或文件路径读内容、type=list 列句柄或按名称/路径过滤。",
		map[string]any{
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"memory", "handle", "list"},
				"description": "memory=检索长期记忆；handle=按句柄或文件路径读取文件内容；list=列出句柄",
			},
			"query": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "memory 填 1-4 个搜索关键词；handle 填句柄或文件名 / 路径；list 留空列全部，填类型码 D/I/G/A/V/U 只列该类型，填文件名 / 路径则按名称与路径过滤",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "返回结果数量，默认 5",
			},
		}, []string{"type"}),
}

var scheduleTools = []ToolCall{
	functionTool("schedule", "创建、编辑、删除与查询计划项",
		map[string]any{
			"action":   map[string]any{"type": "string", "enum": []string{"create", "edit", "delete", "query"}, "description": "要执行的动作"},
			"id":       map[string]any{"type": "string", "description": "计划项ID（edit/delete 必填）"},
			"time":     map[string]any{"type": "string", "description": "计划执行时间点（create 必填）"},
			"content":  map[string]any{"type": "string", "description": "计划工作内容（create 必填）"},
			"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "查询关键词数组"},
			"date":     map[string]any{"type": "string", "description": "查询日期范围"},
			"scope":    map[string]any{"type": "string", "enum": []string{"day", "month", "year", "all"}, "description": "查询粒度"},
		}, []string{"action"}),
}

func functionTool(name, description string, properties map[string]any, required []string) ToolCall {
	return ToolCall{Type: "function", Function: ToolDefinition{
		Name:        name,
		Description: description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
	}}
}

// ==== 工具注册 ====

func registerTool(name string, handler ToolHandler) {
	toolMu.Lock()
	toolFunctions[name] = handler
	toolMu.Unlock()
}

func lookupTool(name string) ToolHandler {
	toolMu.RLock()
	defer toolMu.RUnlock()
	return toolFunctions[name]
}

// ==== 智能体控制 ====

// handleDispatchTechnique 技法创作聚合工具：按 type 分派给对应子智能体
func handleDispatchTechnique(args map[string]any) ToolResult {
	kind := strings.ToLower(strings.TrimSpace(stringArg(args, "type")))
	instruction := strings.TrimSpace(stringArg(args, "instruction"))
	switch kind {
	case techniquePainter:
		return dispatchPainter(instruction, stringArg(args, "reference_tag"))
	case techniqueMusician:
		return dispatchMusician(instruction)
	case techniqueScreenshoter:
		return dispatchScreenshoter(instruction)
	case techniqueFileEditor:
		return dispatchFileEditor(instruction)
	default:
		return ToolResult{Text: "技法创作调度失败：type 只能取 painter / musician / screenshoter / file_editor"}
	}
}

// dispatchFileEditor 把自然语言文件操作需求交给「文件编辑」智能体执行。
func dispatchFileEditor(instruction string) ToolResult {
	if instruction == "" {
		return ToolResult{Text: "文件编辑任务调度失败：操作要求不能为空"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), fileEditorTimeout)
	defer cancel()
	result, err := FileEditor.Execute(ctx, instruction)
	if err != nil {
		return ToolResult{Text: "文件编辑任务失败：" + err.Error()}
	}
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "文件编辑已完成"
	}
	if len(result.Handles) > 0 {
		summary += "（资源句柄：" + strings.Join(result.Handles, "、") + "）"
	}
	return ToolResult{Text: summary}
}

func dispatchPainter(description, referenceTag string) ToolResult {
	if description == "" {
		return ToolResult{Text: "绘画任务调度失败：创作描述不能为空"}
	}
	if painterRole == nil {
		return ToolResult{Text: "绘画任务调度失败：绘制者子智能体未就绪"}
	}
	var reference *ImageReference
	if tag := strings.TrimSpace(referenceTag); tag != "" {
		handle, source, desc, ok := resolveImageRef(tag)
		if !ok {
			return ToolResult{Text: "绘画任务调度失败：图片句柄 " + tag + " 不存在，请用 fetch_resources（type=\"list\", query=[\"I\"]）查看可引用的图片句柄后重试"}
		}
		if source == "" {
			note := ""
			if desc != "" {
				note = "：" + desc
			}
			return ToolResult{Text: "绘画任务调度失败：" + handle + " 的图片本体已从内容仓库回收（仅存描述" + note + "），请让用户重新发送该图片"}
		}
		reference = &ImageReference{Handle: handle, Source: source, Desc: desc}
	}
	result := painterRole.createCreativeWork(description, reference)
	return ToolResult{Text: result}
}

func dispatchMusician(description string) ToolResult {
	if description == "" {
		return ToolResult{Text: "音乐任务调度失败：创作描述不能为空"}
	}
	if musicianRole == nil {
		return ToolResult{Text: "音乐任务调度失败：演奏者子智能体未就绪"}
	}
	return ToolResult{Text: musicianRole.createCreativeWork(description)}
}

func dispatchScreenshoter(requirement string) ToolResult {
	if requirement == "" {
		return ToolResult{Text: "截图任务调度失败：截图要求不能为空"}
	}
	if screenshoterRole == nil {
		return ToolResult{Text: "截图任务调度失败：截图者子智能体未就绪"}
	}
	result := screenshoterRole.createCreativeWork(requirement)
	return ToolResult{Text: result, Image: screenshoterRole.takePrimaryImage()}
}

// ==== 记忆检索 ====

// fetchMemory 记忆模式：只检索长期记忆库，不混入资源目录或句柄查询。
func fetchMemory(keywords []string, limit int) ToolResult {
	if len(keywords) == 0 {
		return ToolResult{Text: "fetch_resources(memory) 失败：query 需要 1-4 个搜索关键词"}
	}
	topK := limit
	if topK <= 0 {
		topK = 5
	}
	if topK > 30 {
		topK = 30
	}
	exclude := []string(nil)
	if !ensureMemoryReady() {
		return ToolResult{Text: "记忆库未就绪，无法检索。请稍后再试一次。"}
	}
	vectorHits := vectorSearch(keywords, topK, exclude)
	knownIDs := map[string]bool{}
	for _, hit := range vectorHits {
		knownIDs[hit.ID] = true
	}
	fallbackHits := []MemorySearchHit{}
	if len(vectorHits) < topK {
		fallbackHits = keywordFallback(keywords, topK, exclude, knownIDs)
	}
	hits := append(vectorHits, fallbackHits...)
	if len(hits) > topK {
		hits = hits[:topK]
	}
	if len(hits) == 0 {
		return ToolResult{Text: "记忆库中没有相关记录，可以换一种表述再试一次；"}
	}
	lines := make([]string, 0, len(hits))
	for i, hit := range hits {
		meta := fmt.Sprintf("角色=%s｜时间=%s｜相关度=%.3f", roleLabel(hit.Role), formatMemoryTime(hit.Timestamp), hit.Similarity)
		lines = append(lines, fmt.Sprintf("[%d] %s\n%s", i+1, meta, previewText(hit.Content, 400)))
	}
	viaFallback := ""
	if len(fallbackHits) > 0 && len(vectorHits) < topK {
		viaFallback = fmt.Sprintf("（语义检索 %d 条 + 关键词补足 %d 条）", len(vectorHits), len(fallbackHits))
	}
	text := fmt.Sprintf("检索到 %d 条相关记忆%s：\n%s", len(hits), viaFallback, strings.Join(lines, "\n\n"))
	text += "\n\n只依据以上原文回答；原文不足以回答用户问题时，直接说明不记得/没有找到相关信息，不要编造，也不要改用 handle 模式现场读取。"
	return ToolResult{Text: text}
}

func roleLabel(role string) string {
	switch role {
	case "user":
		return "用户"
	case "assistant":
		return "月华"
	case "system":
		return "系统"
	case "tool":
		return "工具"
	case "image":
		return "图片"
	}
	if role == "" {
		return "未知"
	}
	return role
}

func formatMemoryTime(timestamp int64) string {
	if timestamp <= 0 {
		return "时间未知"
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

func previewText(text string, limit int) string {
	return truncateRunes(text, limit)
}

func vectorSearch(keywords []string, topK int, exclude []string) []MemorySearchHit {
	merged := map[string]MemorySearchHit{}
	order := make([]string, 0)
	for _, keyword := range keywords {
		results, err := memory.Query(context.Background(), keyword, "text", topK, memory.QueryOptions{})
		if err != nil {
			continue
		}
		for _, item := range memoryResults(results) {
			content := strings.TrimSpace(item.Content)
			if content == "" || containsAny(content, normalizeTerms(exclude)) || item.ID == "" {
				continue
			}
			item.Content = content
			if existing, ok := merged[item.ID]; !ok {
				merged[item.ID] = item
				order = append(order, item.ID)
			} else if item.Similarity > existing.Similarity {
				merged[item.ID] = item
			}
		}
	}
	hits := make([]MemorySearchHit, 0, len(order))
	for _, id := range order {
		hits = append(hits, merged[id])
	}
	sortMemoryHits(hits)
	return hits
}

func sortMemoryHits(hits []MemorySearchHit) {
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Similarity > hits[j-1].Similarity; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
}

func keywordFallback(keywords []string, topK int, exclude []string, known map[string]bool) []MemorySearchHit {
	terms := make([]string, 0)
	seen := map[string]bool{}
	for _, keyword := range keywords {
		text := strings.TrimSpace(keyword)
		if text == "" {
			continue
		}
		if len([]rune(text)) <= 20 {
			if !seen[text] {
				seen[text] = true
				terms = append(terms, text)
			}
			continue
		}
		for _, part := range strings.Fields(text) {
			word := strings.TrimSpace(part)
			if len([]rune(word)) >= 2 && !seen[word] {
				seen[word] = true
				terms = append(terms, word)
			}
		}
	}
	if len(terms) == 0 {
		return nil
	}
	if len(terms) > 8 {
		terms = terms[:8]
	}
	type entry struct {
		hit      MemorySearchHit
		hitCount int
	}
	merged := map[string]entry{}
	order := make([]string, 0)
	for _, term := range terms {
		results, err := memory.KeywordSearch("text", []string{term}, false, topK*2)
		if err != nil {
			continue
		}
		for _, item := range memoryResults(results) {
			content := strings.TrimSpace(item.Content)
			if content == "" || containsAny(content, normalizeTerms(exclude)) || item.ID == "" || known[item.ID] {
				continue
			}
			item.Content = content
			if existing, ok := merged[item.ID]; ok {
				existing.hitCount++
				merged[item.ID] = existing
			} else {
				merged[item.ID] = entry{hit: item, hitCount: 1}
				order = append(order, item.ID)
			}
		}
	}
	hits := make([]MemorySearchHit, 0, len(order))
	for _, id := range order {
		hits = append(hits, merged[id].hit)
	}
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0; j-- {
			left, right := merged[hits[j-1].ID], merged[hits[j].ID]
			if right.hitCount > left.hitCount || (right.hitCount == left.hitCount && right.hit.Timestamp > left.hit.Timestamp) {
				hits[j], hits[j-1] = hits[j-1], hits[j]
			}
		}
	}
	return hits
}

// ==== 资源工具 ====

// handleFetchResources 统一资源入口：memory / handle / list。
func handleFetchResources(args map[string]any) ToolResult {
	mode := strings.ToLower(strings.TrimSpace(stringArg(args, "type")))
	query := stringListArg(args, "query")
	limit := int(numberArg(args, "limit"))
	if limit <= 0 {
		limit = 5
	}
	switch mode {
	case "memory":
		return fetchMemory(query, limit)
	case "handle":
		return fetchHandles(query, limit)
	case "list":
		return fetchList(query, limit)
	default:
		return ToolResult{Text: "fetch_resources 失败：type 只能取 memory / handle / list"}
	}
}

// fetchHandles 句柄模式：按句柄返回完整信息与可读内容。
func fetchHandles(handles []string, limit int) ToolResult {
	if len(handles) == 0 {
		return ToolResult{Text: "fetch_resources(handle) 失败：query 需要句柄，例如 [\"@D.003\"]"}
	}
	if limit > 0 && len(handles) > limit {
		handles = handles[:limit]
	}
	blocks := make([]string, 0)
	missing := make([]string, 0)
	for _, raw := range handles {
		if block := readHandleBlock(raw, "", &missing); block != "" {
			blocks = append(blocks, block)
		}
	}
	return finishResourceBlocks(blocks, missing)
}

// fetchList 列表模式：query 为空列出全部；首项为类型码时只列该类型；
// 其余项按文件名 / 纯文件夹名 / 路径过滤（多项取并集）。
func fetchList(query []string, limit int) ToolResult {
	kind := ""
	rest := query
	if len(query) > 0 {
		if code := strings.ToUpper(strings.TrimSpace(query[0])); isKnownKind(code) {
			kind = code
			rest = query[1:]
		}
	}
	keywords := make([]string, 0, len(rest))
	for _, raw := range rest {
		if keyword := normalizeListKeyword(raw); keyword != "" {
			keywords = append(keywords, keyword)
		}
	}
	if limit > 200 {
		limit = 200
	}
	if len(keywords) == 0 {
		return ToolResult{Text: listResourcesText(kind, limit)}
	}
	return ToolResult{Text: listResourcesTextMatching(kind, keywords, limit)}
}

// normalizeListKeyword 归一化过滤词：去空白、反斜杠转正斜杠、去首尾路径分隔符
func normalizeListKeyword(raw string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"), "/")
}

// readHandleBlock 单个句柄的取回 / 解读
//
// 顺序严格遵循规范三：已有解析文本直接返回；否则按类型分流到 Descriptor /
// audio_transcriber / WebSearch 直驱 / B 站直驱。
func readHandleBlock(raw, ask string, missing *[]string) string {
	parsed := parseHandle(raw)
	if parsed == nil {
		// 非句柄输入：复用统一文件定位器，按文件名 / 路径定位后再登记句柄
		match, resolveErr := locator.ResolveToHandle(raw, nil)
		if resolveErr != nil {
			*missing = append(*missing, raw+"（"+resolveErr.Error()+"）")
			return ""
		}
		parsed = parseHandle(match.Handle)
	}
	if parsed == nil {
		*missing = append(*missing, raw)
		return ""
	}
	normalized := formatHandle(*parsed)
	entry := getResourceEntry(normalized)
	if entry == nil {
		*missing = append(*missing, normalized)
		return ""
	}
	if entry.State == "lost" {
		deleteResource(normalized)
		return resourceBlockHeader(normalized, entry) + lostFileNotice
	}
	// 压缩包 / GGUF 等归档类实体：不按文本取回，交给归档解析智能体
	if ArchiveReader.IsArchiveExt(entryExt(entry)) || ArchiveReader.IsGGUFExt(entryExt(entry)) {
		return archiveHandleBlock(normalized, entry, ask)
	}
	// 1. 解析文本优先（文件类直接读原文，不在此短路）
	if parsed.Kind != resource.KindDocument &&
		entry.ParseState == resource.ParseReady && strings.TrimSpace(entry.ParseText) != "" {
		return resourceBlockHeader(normalized, entry) + entry.ParseText
	}

	switch parsed.Kind {
	case resource.KindImage, resource.KindAnimated:
		return imageHandleBlock(normalized, entry, ask)
	case resource.KindAudio:
		return audioHandleBlock(normalized, entry)
	case resource.KindVideo:
		return videoHandleBlock(normalized, entry, ask)
	case resource.KindURL:
		return webHandleBlock(normalized, entry, ask)
	default:
		return documentHandleBlock(normalized, entry, ask, missing)
	}
}

// imageHandleBlock 图片 / 动图：本地已有副本直接解读；远程直链先下载再解读
func imageHandleBlock(handle string, entry *ResourceEntry, ask string) string {
	originalURL := entry.URL
	if entry.BlobPath == "" && resource.IsRemoteURL(entry.URL) {
		if admitted := admitMedia(entry.URL, mediaName(entry.URL, ".gif"), entry.Origin, entry.Kind); admitted != nil && admitted.Handle != "" {
			if local := getResourceEntry(admitted.Handle); local != nil {
				handle, entry = admitted.Handle, local
			}
		}
	}
	desc := describeResource(handle, ask)
	if desc == "" {
		if getResourceEntry(handle) == nil {
			return resourceBlockHeader(handle, entry) + lostFileNotice
		}
		desc = "（暂无描述，且本体不可直接解读）"
	}
	// 远程来源的解析文本同时写回原 URL，保证同一链接下次直接命中
	if originalURL != "" && resource.IsRemoteURL(originalURL) {
		_ = resource.SetParseText(originalURL, desc)
	}
	return resourceBlockHeader(handle, entry) + desc
}

// audioHandleBlock 音频：读取接入阶段写入的转写描述（无实体音频可解码）
func audioHandleBlock(handle string, entry *ResourceEntry) string {
	text := audioParseTextOf(handle)
	if strings.TrimSpace(text) == "" {
		text = ASRFallbackText
	}
	return resourceBlockHeader(handle, entry) + text
}

// videoHandleBlock 视频：B 站走直驱印象；本地或远程直链走 Descriptor
func videoHandleBlock(handle string, entry *ResourceEntry, ask string) string {
	rawURL := entry.URL
	if resource.BilibiliBvid(rawURL) != "" {
		impression, err := bilibiliImpressionHook(rawURL, ask)
		if err != nil {
			return resourceBlockHeader(handle, entry) + "视频印象获取失败：" + err.Error()
		}
		if strings.TrimSpace(impression) != "" {
			_ = resource.SetParseText(rawURL, impression)
		}
		return resourceBlockHeader(handle, entry) + impression
	}
	source := rawURL
	if entry.BlobPath == "" || !resource.IsLocalURL(rawURL) {
		admitted := admitMedia(rawURL, mediaName(rawURL, ".mp4"), entry.Origin, resource.KindVideo)
		if admitted == nil || admitted.Handle == "" {
			return resourceBlockHeader(handle, entry) + "视频读取失败：本地副本未能下载落盘"
		}
		source = admitted.URL
	}
	desc, err := describeVideoURL(source, ask)
	if err != nil {
		if errors.Is(err, Vision.ErrVideoExpired) {
			// 远程地址失效且本地无副本：重新下载一次
			if admitted := admitMedia(rawURL, mediaName(rawURL, ".mp4"), entry.Origin, resource.KindVideo); admitted != nil {
				if retry, retryErr := describeVideoURL(admitted.URL, ask); retryErr == nil {
					return resourceBlockHeader(handle, entry) + retry
				}
			}
			return resourceBlockHeader(handle, entry) + "这段视频的远程地址已经失效，本地也没有可复用的副本；请对方重新发送一次视频。"
		}
		return resourceBlockHeader(handle, entry) + "视频读取失败：" + err.Error()
	}
	if desc == "" {
		desc = "（视频暂无可用描述）"
	}
	if entry.URL != "" {
		_ = resource.SetParseText(entry.URL, desc)
	}
	return resourceBlockHeader(handle, entry) + desc
}

// webHandleBlock 网页：本地已有视频/动图副本走 Descriptor；否则 WebSearch 直驱
func webHandleBlock(handle string, entry *ResourceEntry, ask string) string {
	rawURL := entry.URL
	if resource.BilibiliBvid(rawURL) != "" {
		impression, err := bilibiliImpressionHook(rawURL, ask)
		if err != nil {
			return resourceBlockHeader(handle, entry) + "视频印象获取失败：" + err.Error()
		}
		if strings.TrimSpace(impression) != "" {
			_ = resource.SetParseText(rawURL, impression)
		}
		return resourceBlockHeader(handle, entry) + impression
	}
	ext := strings.ToLower(filepath.Ext(pathPartOf(rawURL)))
	if resource.KindOfExt(ext) == resource.KindVideo || (resource.MayBeAnimated(ext) && resource.KindOfExt(ext) == resource.KindImage) {
		admitted := admitMedia(rawURL, mediaName(rawURL, ext), entry.Origin, "")
		if admitted != nil && admitted.URL != "" {
			desc, err := describeVideoURL(admitted.URL, ask)
			if err == nil && strings.TrimSpace(desc) != "" {
				_ = resource.SetParseText(rawURL, desc)
				return resourceBlockHeader(handle, entry) + desc
			}
		}
	}
	title, summary, err := readPageHook(rawURL, ask)
	if err != nil {
		return resourceBlockHeader(handle, entry) + "这个链接现在打不开：" + err.Error()
	}
	if strings.TrimSpace(summary) == "" {
		return resourceBlockHeader(handle, entry) + "（这个链接没有读取出可用内容）"
	}
	_ = resource.SetParseText(rawURL, summary)
	if strings.TrimSpace(title) != "" {
		_, _ = resource.AdmitURL(resource.URLAdmitInput{URL: rawURL, Title: title, Origin: entry.Origin})
	}
	return resourceBlockHeader(handle, entry) + summary
}

// documentHandleBlock 文件：直接取原文；带 ask 时交给文献解析智能体探查
func documentHandleBlock(handle string, entry *ResourceEntry, ask string, missing *[]string) string {
	if ask != "" {
		answer := inspectTextResource(handle, ask)
		if answer == "" {
			return resourceBlockHeader(handle, entry) + "探查失败：文献解析没有给出答复，可去掉 ask 直接取原文"
		}
		return resourceBlockHeader(handle, entry) + answer
	}
	result := readResource(handle, 12000)
	if result == nil {
		*missing = append(*missing, handle)
		return ""
	}
	if result.Lost {
		deleteResource(handle)
		return resourceBlockHeader(handle, entry) + lostFileNotice
	}
	rangeNote := ""
	if result.LineStart > 0 && result.LineEnd > 0 {
		rangeNote = fmt.Sprintf("｜第 %d-%d 行", result.LineStart, result.LineEnd)
	}
	sectionNote := ""
	if result.SectionTitle != "" {
		sectionNote = "｜" + result.SectionTitle
	}
	truncated := ""
	if result.Truncated {
		truncated = "\n（内容较长已截断；可带 |L起-止 或 |§n 定向读取）"
	}
	return fmt.Sprintf("【%s%s%s｜共 %d 行】\n%s%s", handle, sectionNote, rangeNote, result.TotalLines, result.Content, truncated)
}

// archiveHandleBlock 压缩包 / GGUF：交给归档解析智能体做确定性解析
func archiveHandleBlock(handle string, entry *ResourceEntry, ask string) string {
	if strings.TrimSpace(entry.BlobPath) == "" {
		return resourceBlockHeader(handle, entry) + "（该归档实体没有可读取的本地副本）"
	}
	text, err := ArchiveReader.Inspect(entry.BlobPath, ask)
	if err != nil {
		return resourceBlockHeader(handle, entry) + "解析失败：" + err.Error()
	}
	return resourceBlockHeader(handle, entry) + text
}

// entryExt 资源扩展名（记录为空时按文件名推导）
func entryExt(entry *ResourceEntry) string {
	if ext := strings.TrimSpace(entry.Ext); ext != "" {
		return strings.ToLower(ext)
	}
	return strings.ToLower(filepath.Ext(entry.Name))
}

// resourceBlockHeader 统一的资源块标题
func resourceBlockHeader(handle string, entry *ResourceEntry) string {
	kind := kindFromNormalized(handle)
	title := strings.TrimSpace(entry.Title)
	if title == "" {
		title = strings.TrimSpace(entry.Name)
	}
	if title == "" {
		title = handle
	}
	return "【" + handle + "｜" + kindLabel(kind) + "｜" + title + "】\n"
}

// kindFromNormalized 从规范句柄取类型码
func kindFromNormalized(handle string) string {
	if parsed := parseHandle(handle); parsed != nil {
		return parsed.Kind
	}
	return ""
}

func finishResourceBlocks(blocks, missing []string) ToolResult {
	if len(blocks) == 0 {
		return ToolResult{Text: "取回失败：" + strings.Join(missing, "、") + " —— " + handleInvalidNotice + "。请先用 fetch_resources（type=\"list\"）确认可用句柄"}
	}
	text := strings.Join(blocks, "\n\n---\n\n")
	if len(missing) > 0 {
		text += "\n\n以下句柄无法取回：" + strings.Join(missing, "、") + "（" + handleInvalidNotice + "）"
	}
	return ToolResult{Text: text}
}

func listResourcesText(kind string, limit int) string {
	return renderResourceList(listResources(kind, limit), "当前可用资源", "资源目录为空：当前没有已收录的资源")
}

// listResourcesTextMatching 过滤模式：按文件名 / 路径列出匹配的句柄
func listResourcesTextMatching(kind string, keywords []string, limit int) string {
	hint := "没有匹配「" + strings.Join(keywords, "、") + "」的资源；可用 fetch_resources（type=\"list\"）查看全部目录"
	return renderResourceList(listResourcesMatching(kind, keywords, limit), "匹配的资源", hint)
}

// renderResourceList 目录统一格式化：自动清理失效句柄，输出句柄 / 类型 / 标题 / 摘要
func renderResourceList(entries []ResourceEntry, head, emptyHint string) string {
	valid := make([]ResourceEntry, 0, len(entries))
	purged := 0
	for _, entry := range entries {
		if entry.State == "lost" {
			if deleteResource(entry.Handle) {
				purged++
			}
			continue
		}
		valid = append(valid, entry)
	}
	if len(valid) == 0 {
		note := ""
		if purged > 0 {
			note = fmt.Sprintf("（已自动清理 %d 条源文件丢失的失效句柄）", purged)
		}
		return emptyHint + note + "。"
	}
	stats := resourceStatistics()
	lines := make([]string, 0, len(valid))
	for _, entry := range valid {
		desc := ""
		if entry.Summary != "" {
			desc = "：" + previewText(strings.Join(strings.Fields(entry.Summary), " "), 80)
		}
		sectionNote := ""
		if entry.SectionCount > 0 {
			sectionNote = fmt.Sprintf(" %d节", entry.SectionCount)
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s%s%s", entry.Handle, kindLabel(entry.Kind), entry.Title, sectionNote, desc))
	}
	purgeNote := ""
	if purged > 0 {
		purgeNote = fmt.Sprintf("；已自动清理 %d 条源文件丢失的失效句柄", purged)
	}
	statsNote := ""
	if stats != nil {
		statsNote = fmt.Sprintf("；目录总计 %d 条 / %s", stats.Total, formatBytes(stats.Bytes))
	}
	header := fmt.Sprintf("%s（共 %d 条，按收录时间倒序，越靠前越新%s%s）：", head, len(valid), purgeNote, statsNote)
	tail := "\n需要内容时用 fetch_resources（type=\"handle\", query=[句柄]）取回。"
	return header + "\n" + strings.Join(lines, "\n") + tail
}

// ==== 计划表工具 ====

func handleScheduleTool(args map[string]any) ToolResult {
	action := strings.TrimSpace(stringArg(args, "action"))
	switch action {
	case "create":
		return scheduleCreate(args)
	case "edit":
		return scheduleEdit(args)
	case "delete":
		return scheduleDelete(args)
	case "query":
		scheduleMu.Lock()
		cache := append([]ScheduleItem(nil), scheduleCache...)
		scheduleMu.Unlock()
		return querySchedules(cache, stringListArg(args, "keywords"), stringArg(args, "date"), stringArg(args, "scope"))
	default:
		return ToolResult{Text: "计划表工具调用失败：未知的动作 \"" + action + "\"，action 必须是 create / edit / delete / query 之一"}
	}
}

func scheduleCreate(args map[string]any) ToolResult {
	rawTime := strings.TrimSpace(stringArg(args, "time"))
	content := strings.TrimSpace(stringArg(args, "content"))
	if rawTime == "" {
		return ToolResult{Text: "创建计划项失败：执行时间不能为空"}
	}
	if content == "" {
		return ToolResult{Text: "创建计划项失败：工作内容不能为空"}
	}
	normalized := normalizeTime(rawTime)
	if normalized == "" {
		return ToolResult{Text: "创建计划项失败：无法解析时间格式 \"" + rawTime + "\""}
	}
	item := ScheduleItem{ID: generateScheduleID(), Time: normalized, Content: content}
	scheduleMu.Lock()
	scheduleCache = append(scheduleCache, item)
	ok := saveSchedulesToDisk(scheduleCache)
	if !ok {
		scheduleCache = scheduleCache[:len(scheduleCache)-1]
	}
	scheduleMu.Unlock()
	if !ok {
		return ToolResult{Text: "创建计划项失败：保存到磁盘时出错"}
	}
	return ToolResult{Text: fmt.Sprintf("计划项创建成功：ID为 %s，执行时间: %s，内容: %s", item.ID, item.Time, item.Content)}
}

func scheduleEdit(args map[string]any) ToolResult {
	id := strings.TrimSpace(stringArg(args, "id"))
	rawTime := strings.TrimSpace(stringArg(args, "time"))
	content := strings.TrimSpace(stringArg(args, "content"))
	if id == "" {
		return ToolResult{Text: "编辑计划项失败：计划项ID不能为空"}
	}
	if rawTime == "" && content == "" {
		return ToolResult{Text: "编辑计划项失败：至少需要提供 time 或 content 中的一个进行修改"}
	}
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	index := -1
	for i := range scheduleCache {
		if scheduleCache[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ToolResult{Text: "编辑计划项失败：未找到ID为 " + id + " 的计划项"}
	}
	snapshot := scheduleCache[index]
	if rawTime != "" {
		normalized := normalizeTime(rawTime)
		if normalized == "" {
			return ToolResult{Text: "编辑计划项失败：无法解析时间格式 \"" + rawTime + "\""}
		}
		scheduleCache[index].Time = normalized
	}
	if content != "" {
		scheduleCache[index].Content = content
	}
	if !saveSchedulesToDisk(scheduleCache) {
		scheduleCache[index] = snapshot
		return ToolResult{Text: "编辑计划项失败：保存到磁盘时出错"}
	}
	return ToolResult{Text: fmt.Sprintf("计划项编辑成功：ID为 %s，已更新为 执行时间: %s，内容: %s", id, scheduleCache[index].Time, scheduleCache[index].Content)}
}

func scheduleDelete(args map[string]any) ToolResult {
	id := strings.TrimSpace(stringArg(args, "id"))
	if id == "" {
		return ToolResult{Text: "删除计划项失败：计划项ID不能为空"}
	}
	scheduleMu.Lock()
	defer scheduleMu.Unlock()
	index := -1
	for i := range scheduleCache {
		if scheduleCache[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return ToolResult{Text: "删除计划项失败：未找到ID为 " + id + " 的计划项"}
	}
	deleted := scheduleCache[index]
	scheduleCache = append(scheduleCache[:index], scheduleCache[index+1:]...)
	if !saveSchedulesToDisk(scheduleCache) {
		scheduleCache = append(scheduleCache[:index], append([]ScheduleItem{deleted}, scheduleCache[index:]...)...)
		return ToolResult{Text: "删除计划项失败：保存到磁盘时出错"}
	}
	return ToolResult{Text: fmt.Sprintf("计划项删除成功：已移除 [%s] %s - %s", deleted.ID, deleted.Time, deleted.Content)}
}

// registerDialogueTools 注册对话者可用的全部工具
func registerDialogueTools() {
	registerTool(techniqueToolName, handleDispatchTechnique)
	registerTool("fetch_resources", handleFetchResources)
	registerTool("schedule", handleScheduleTool)
	registerTool(networkExploreToolName, handleNetworkExplore)
	registerTool(crystalToolName, handleUseTheProgram)
	logger.Info(lunarCoreModule, "[工具] 对话者工具已注册")
}
