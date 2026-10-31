package LunarPersonality

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/resource"
)

// newPainterRole 创建绘制者角色
func newPainterRole() *PainterRole {
	prompts := newPainterPromptLibrary()
	lunar := prompts.Builtin["月华"].Appearance
	crystal := prompts.Builtin["琉璃"].Appearance
	builder := newModelBuilder(composePainterPrompt(lunar, crystal), "绘制者", 40)
	return &PainterRole{
		ModelBuilder:     builder,
		Prompts:          prompts,
		CustomCharacters: loadCustomCharacters(),
	}
}

func (role *PainterRole) lunarAppearance() string {
	return role.Prompts.Builtin["月华"].Appearance
}

func (role *PainterRole) crystalAppearance() string {
	return role.Prompts.Builtin["琉璃"].Appearance
}

// createCreativeWork 创作任务入口
func (role *PainterRole) createCreativeWork(taskDescription string, reference *ImageReference) string {
	role.CustomCharacters = loadCustomCharacters()
	role.bindReference(reference)
	details := make([]PaintingDetail, 0)
	taskContent := role.buildTaskContent(taskDescription)
	tools := role.toolDefinitions()
	return runCreativeLoop("绘制者", role.ModelBuilder, taskContent, tools, 3,
		func(call ToolCallItem) string { return role.executeTool(call) },
		func(call ToolCallItem) { collectPaintingDetail(call, &details) },
		func() string { return buildPaintingSummary(details) },
	)
}

// toolDefinitions 获取工具定义（角色目录动态替换）
func (role *PainterRole) toolDefinitions() []ToolCall {
	catalog := role.characterCatalog()
	tools := painterToolDefinitions()
	for i := range tools {
		tools[i].Function.Description = strings.ReplaceAll(tools[i].Function.Description, "{characters}", catalog)
		if parameters, ok := tools[i].Function.Parameters.(map[string]any); ok {
			replaceCharacterPlaceholder(parameters, catalog)
		}
	}
	return tools
}

func replaceCharacterPlaceholder(parameters map[string]any, catalog string) {
	properties, _ := parameters["properties"].(map[string]any)
	for key, value := range properties {
		switch item := value.(type) {
		case map[string]any:
			if description, ok := item["description"].(string); ok {
				item["description"] = strings.ReplaceAll(description, "{characters}", catalog)
			}
		case string:
			properties[key] = strings.ReplaceAll(item, "{characters}", catalog)
		}
	}
}

func (role *PainterRole) characterCatalog() string {
	profiles := mergeProfilesWithBuiltin(role.Prompts.Builtin, role.CustomCharacters)
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	if len(names) == 0 {
		return "当前可用角色：通用-女 / 通用-男（临时描述外观）"
	}
	return "当前可用角色：" + strings.Join(names, "、") + "；也可用 通用-女 / 通用-男 临时描述外观"
}

// executeTool 执行绘画工具调用
func (role *PainterRole) executeTool(call ToolCallItem) string {
	args := parseToolArgs(call.Function.Arguments)
	switch call.Function.Name {
	case "diffusion_generation":
		return role.handleDiffusionGeneration(args)
	case "image_to_image":
		return role.handleImageToImage(args)
	case "portrait":
		return role.handlePortrait(args)
	case "create_character":
		return role.handleCreateCharacter(args)
	case "group_photo":
		return role.handleGroupPhoto(args)
	case "switch_diffusion_scheme":
		return role.handleSwitchDiffusionScheme(args)
	case "list_images":
		return role.handleListImages()
	case "upscale_image":
		return role.handleUpscaleImage(args)
	default:
		return "未知工具: " + call.Function.Name + "，可用工具为 diffusion_generation、image_to_image、portrait、create_character、group_photo、switch_diffusion_scheme、list_images、upscale_image"
	}
}

// paint 执行图像生成并推送至前端
func (role *PainterRole) paint(params GenerateImageParams, label string) string {
	result, err := multimodal.GenerateImage(
		params.Prompt, params.NegativePrompt, 1, params.Width, params.Height,
		params.Steps, params.Strength, params.CfgScale, params.Seed, params.InitImg, params.SuperResolution,
	)
	if err != nil {
		logger.Error(lunarCoreModule, "[绘制者] %s失败: %v", label, err)
		return label + "失败: " + err.Error()
	}
	base64Data, _ := result["base64"].(string)
	if base64Data == "" {
		return label + "失败：引擎返回空结果"
	}
	path, _ := result["path"].(string)
	width := toIntValue(result["width"])
	height := toIntValue(result["height"])
	seed := toInt64Value(result["seed"])
	role.ResultPath = path
	url := localPathToReadURL(path)
	if url != "" {
		urls := pushAssistantImage([]string{url}, false)
		if len(urls) > 0 {
			role.ResultPath = urls[0]
			role.rememberRuntimeArtwork(urls[0], "最新作品")
		} else {
			role.rememberRuntimeArtwork(url, "最新作品")
		}
	}
	stepsNote := ""
	if params.Steps > 0 {
		stepsNote = fmt.Sprintf("，%d 步", params.Steps)
	}
	extra := ""
	if params.SuperResolution {
		extra = "，已附带超分辨率重建"
	}
	return fmt.Sprintf("%s成功。图片尺寸: %dx%d，seed: %d%s%s", label, width, height, seed, stepsNote, extra)
}

// bindReference 绑定本次任务的参考图。
//
// 没有显式 reference_tag 时自动沿用最近一次产出的画作：这样「把刚才那张图改成…」
// 这类跨流程请求能直接以刚生成的图为图生图基座，而不是误判成「没有参考图」。
func (role *PainterRole) bindReference(reference *ImageReference) {
	role.Reference = ReferenceState{}
	role.referenceFromRuntime = false
	if reference == nil || reference.Source == "" {
		if role.RuntimeArtwork.Image != "" {
			role.Reference = role.RuntimeArtwork
			role.referenceFromRuntime = true
			logger.Info(lunarCoreModule, "[绘制者] 本次任务未指定参考图，自动沿用最近产出画作: %s", role.RuntimeArtwork.Image)
		}
		return
	}
	role.Reference.Handle = reference.Handle
	role.Reference.Image = reference.Source
	_, _, width, height, err := visionCopy(reference.Source, reference.Handle, 0)
	vision := visionReference(reference.Handle, reference.Source)
	if err != nil || vision == "" {
		logger.Warn(lunarCoreModule, "[绘制者] 参考图视觉副本生成失败: %v", err)
		return
	}
	role.Reference.Vision = vision
	role.Reference.Width = width
	role.Reference.Height = height
	// 本次任务指定了参考图：运行时画作先刷新为该图，直到产出新作品。
	role.setRuntimeImage(reference.Handle, vision, "本次参考图")
}

// rememberRuntimeArtwork 把最新产出登记为运行时画作（句柄缩影版），
// 同时更新图生图基座：同一创作流程后续的局部修改会直接以这张新图为起点。
//
// 运行时消息会在每次模型请求前自动插入，不占上下文窗口配额；
// 这样对话者反复转述局部修改时，画面基准不会随历史消息被裁掉。
func (role *PainterRole) rememberRuntimeArtwork(source, note string) {
	rel := localPathToRel(source)
	if rel == "" {
		return
	}
	// 先走视觉副本管线，确保缩影存在并按句柄叠印
	_, _, width, height, err := visionCopy(rel, "", 0)
	if err != nil {
		logger.Warn(lunarCoreModule, "[绘制者] 运行时画作缩影生成失败: %v", err)
		return
	}
	handle := ""
	if entry, ok := resource.ForPath(rel); ok {
		handle = entry.Handle
	}
	vision := visionReference(handle, rel)
	role.RuntimeArtwork = ReferenceState{
		Image:  rel,
		Handle: handle,
		Vision: vision,
		Width:  width,
		Height: height,
	}
	role.Reference = role.RuntimeArtwork
	role.referenceFromRuntime = false
	role.setRuntimeImage(handle, vision, note)
}

// setRuntimeImage 刷新绘制者的运行时画作消息。
func (role *PainterRole) setRuntimeImage(handle, vision, note string) {
	if vision == "" {
		return
	}
	label := "【运行时画作】" + note
	if handle != "" {
		label += "（" + handle + "）"
	}
	label += "。局部修改、延续风格或调整细节时，以这张图作为当前画面基准，不要凭记忆重画。"
	role.RuntimeMessages = []PostMessage{{
		Role: RoleUser,
		Content: []ContentItem{
			{Type: ContentText, Text: label},
			{Type: ContentImageURL, ImageURL: &URLPayload{URL: vision}},
		},
	}}
}

// localPathToRel 把 /file/read/ 地址或本地路径归一化为 multimedia/ 相对路径。
func localPathToRel(source string) string {
	normalized := strings.ReplaceAll(source, "\\", "/")
	if cut := strings.IndexAny(normalized, "?#"); cut >= 0 {
		normalized = normalized[:cut]
	}
	lower := strings.ToLower(normalized)
	if marker := strings.LastIndex(lower, "/multimedia/"); marker >= 0 {
		return normalized[marker+1:]
	}
	if strings.HasPrefix(lower, "multimedia/") {
		return normalized
	}
	return ""
}

func (role *PainterRole) buildTaskContent(taskDescription string) any {
	// 参考图来自运行时画作时，图片已由运行时消息挂载，正文不再重复附图
	if role.Reference.Vision == "" || role.referenceFromRuntime {
		return taskDescription
	}
	return []ContentItem{
		{Type: ContentText, Text: taskDescription},
		{Type: ContentImageURL, ImageURL: &URLPayload{URL: role.Reference.Vision}},
	}
}

func (role *PainterRole) planFor(ratio string) DimensionPlan {
	var reference *DimensionPlan
	if role.Reference.Image != "" && role.Reference.Width > 0 && role.Reference.Height > 0 {
		reference = &DimensionPlan{Width: role.Reference.Width, Height: role.Reference.Height}
	}
	return resolveDimensionPlan(ratio, reference)
}

func (role *PainterRole) referenceStatus() string {
	if role.Reference.Image == "" {
		return "本次任务尚未附带参考图"
	}
	plan := role.planFor("")
	size := ""
	if role.Reference.Width > 0 && role.Reference.Height > 0 {
		size = fmt.Sprintf("（%dx%d）", role.Reference.Width, role.Reference.Height)
	}
	label := role.Reference.Handle
	if label == "" {
		label = role.Reference.Image
	}
	source := ""
	if role.referenceFromRuntime {
		source = "（沿用上一次生成的画作）"
	}
	return fmt.Sprintf("本次任务参考图：%s%s%s，输出尺寸将按参考图比例取 %dx%d", label, size, source, plan.Width, plan.Height)
}

func toIntValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	}
	return 0
}

func toInt64Value(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case int:
		return int64(number)
	case int64:
		return number
	}
	return 0
}
