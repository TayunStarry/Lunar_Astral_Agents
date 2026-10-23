package lunar

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
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

// bindReference 绑定本次任务的参考图
func (role *PainterRole) bindReference(reference *ImageReference) {
	role.Reference = ReferenceState{}
	if reference == nil || reference.Source == "" {
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
}

func (role *PainterRole) buildTaskContent(taskDescription string) any {
	if role.Reference.Vision == "" {
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
	return fmt.Sprintf("本次任务参考图：%s%s，输出尺寸将按参考图比例取 %dx%d", label, size, plan.Width, plan.Height)
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
