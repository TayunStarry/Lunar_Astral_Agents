package Painter

// handlers.go 绘制者的工具执行与作品汇总。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
)

// executeTool 执行绘画工具调用。
func (s *session) executeTool(call chatCall) string {
	args := parseToolArgs(call.Arguments)
	switch call.Name {
	case "diffusion_generation":
		return s.handleDiffusionGeneration(args)
	case "image_to_image":
		return s.handleImageToImage(args)
	case "portrait":
		return s.handlePortrait(args)
	case "create_character":
		return s.handleCreateCharacter(args)
	case "group_photo":
		return s.handleGroupPhoto(args)
	case "switch_diffusion_scheme":
		return s.handleSwitchDiffusionScheme(args)
	case "list_images":
		return s.handleListImages()
	case "upscale_image":
		return s.handleUpscaleImage(args)
	default:
		return "未知工具: " + call.Name + "，可用工具为 diffusion_generation、image_to_image、portrait、create_character、group_photo、switch_diffusion_scheme、list_images、upscale_image"
	}
}

// paint 执行图像生成并推送至前端。
func (s *session) paint(params GenerateImageParams, label string) string {
	result, err := multimodal.GenerateImage(
		params.Prompt, params.NegativePrompt, 1, params.Width, params.Height,
		params.Steps, params.Strength, params.CfgScale, params.Seed, params.InitImg, params.SuperResolution,
	)
	if err != nil {
		logger.Error(module, "[绘制者] %s失败: %v", label, err)
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
	s.resultPath = path
	if url := readURLOf(path); url != "" {
		urls := s.env.PushImages([]string{url})
		if len(urls) > 0 {
			s.resultPath = urls[0]
			s.rememberRuntimeArtwork(urls[0], "最新作品")
		} else {
			s.rememberRuntimeArtwork(url, "最新作品")
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

func (s *session) handleDiffusionGeneration(args map[string]any) string {
	prompt := stringArg(args, "prompt")
	if strings.TrimSpace(prompt) == "" {
		return "扩散生成失败：正向提示词不能为空"
	}
	plan := resolveDimensionPlan(stringArg(args, "ratio"), nil)
	steps := normalizeSteps(numberArg(args, "steps"))
	cfg := numberArg(args, "cfg_scale")
	if cfg <= 0 {
		cfg = 1.0
	}
	return s.paint(GenerateImageParams{
		Prompt:          prompt,
		NegativePrompt:  stringArg(args, "negative_prompt"),
		Width:           plan.Width,
		Height:          plan.Height,
		Steps:           steps,
		CfgScale:        cfg,
		SuperResolution: boolArg(args, "super_resolution"),
	}, "扩散图像生成")
}

func (s *session) handleImageToImage(args map[string]any) string {
	prompt := stringArg(args, "prompt")
	if strings.TrimSpace(prompt) == "" {
		return "图生图失败：正向提示词不能为空"
	}
	if s.reference.Image == "" {
		return "图生图失败：" + s.referenceStatus() + "。图生图基座优先沿用本次任务指定的 reference_tag，其次自动沿用最近一次生成的画作；两者都没有时请改用 diffusion_generation 进行文生图"
	}
	strength := normalizeStrength(numberArg(args, "strength"), "图生图")
	steps := normalizeSteps(numberArg(args, "steps"))
	cfg := resolveImg2ImgCfg(numberArg(args, "cfg_scale"))
	plan := s.planFor("")
	return s.paint(GenerateImageParams{
		Prompt:          prompt,
		NegativePrompt:  stringArg(args, "negative_prompt"),
		Width:           plan.Width,
		Height:          plan.Height,
		Steps:           steps,
		CfgScale:        cfg,
		InitImg:         s.reference.Image,
		Strength:        strength,
		SuperResolution: boolArg(args, "super_resolution"),
	}, "图生图生成")
}

func resolveImg2ImgCfg(value float64) float64 {
	const cfgMin = 0.5
	const cfgMax = 4
	clamp := func(v float64) float64 {
		if v < cfgMin {
			return cfgMin
		}
		if v > cfgMax {
			return cfgMax
		}
		return v
	}
	if config.DiffusionImg2ImgCFG > 0 {
		return clamp(config.DiffusionImg2ImgCFG)
	}
	if value > 0 {
		return clamp(value)
	}
	return 1.0
}

func (s *session) handlePortrait(args map[string]any) string {
	preset := s.portraitPreset(args)
	plan := resolveDimensionPlan(defaultString(stringArg(args, "ratio"), preset.DefaultRatio), nil)
	steps := normalizeSteps(numberArg(args, "steps"))
	cfg := numberArg(args, "cfg_scale")
	if cfg <= 0 {
		cfg = 1.0
	}
	fullPrompt := s.prompts.renderAppearance(preset.Template, AppearanceParts{
		Expression:  stringArg(args, "expression"),
		Posture:     stringArg(args, "posture"),
		Outfit:      stringArg(args, "outfit"),
		Environment: stringArg(args, "environment"),
	}, preset.Outfit, preset.Base, preset.Gender)
	negative := stringArg(args, "negative_prompt")
	if negative == "" {
		negative = s.prompts.Templates["defaultNegative"]
	}
	return s.paint(GenerateImageParams{
		Prompt:          fullPrompt,
		NegativePrompt:  negative,
		Width:           plan.Width,
		Height:          plan.Height,
		Steps:           steps,
		CfgScale:        cfg,
		SuperResolution: boolArg(args, "super_resolution"),
	}, preset.Label+"生成")
}

func (s *session) portraitPreset(args map[string]any) PortraitPreset {
	raw := strings.TrimSpace(stringArg(args, "character"))
	chibi := boolArg(args, "chibi")
	defaultRatioValue := defaultRatio
	if !chibi {
		defaultRatioValue = "2:3"
	}
	templates := s.prompts.Templates
	if raw == "月华" {
		return PortraitPreset{Kind: raw, Chibi: chibi, Label: "月华" + map[bool]string{true: "Q版", false: "全尺寸"}[chibi] + "自画像", Template: pick(chibi, templates["chibiSelf"], templates["fullSelf"]), Base: s.prompts.Builtin["月华"].Appearance, Outfit: pick(chibi, templates["defaultOutfitChibi"], templates["defaultOutfitFull"]), Gender: "女孩", DefaultRatio: defaultRatioValue}
	}
	if raw == "琉璃" {
		return PortraitPreset{Kind: raw, Chibi: chibi, Label: "琉璃" + map[bool]string{true: "Q版", false: "全尺寸"}[chibi] + "立绘", Template: pick(chibi, templates["crystalChibi"], templates["crystalFull"]), Base: s.prompts.Builtin["琉璃"].Appearance, Outfit: pick(chibi, templates["crystalOutfitChibi"], templates["crystalOutfitFull"]), Gender: "女孩", DefaultRatio: defaultRatioValue}
	}
	if custom, ok := s.custom[raw]; ok {
		profile := CharacterProfile{CustomCharacterProfile: custom, Builtin: false}
		return PortraitPreset{Kind: raw, Chibi: chibi, Label: raw + map[bool]string{true: "Q版", false: "全尺寸"}[chibi] + "立绘", Template: pick(chibi, templates["genericChibi"], templates["genericFull"]), Base: profile.Appearance, Outfit: characterOutfit(profile, chibi), Gender: map[bool]string{true: "男孩", false: "女孩"}[profile.Gender == "男"], DefaultRatio: defaultRatioValue}
	}
	male := raw == "通用-男"
	named := ""
	if raw != "" && raw != "通用-女" && raw != "通用-男" && raw != "其他" {
		named = "名为「" + raw + "」的角色"
	}
	appearance := strings.TrimSpace(stringArg(args, "appearance"))
	descParts := make([]string, 0, 2)
	if named != "" {
		descParts = append(descParts, named)
	}
	if appearance != "" {
		descParts = append(descParts, appearance)
	}
	desc := strings.Join(descParts, "，")
	base := templates["femaleBase"]
	if male {
		base = templates["maleBase"]
	}
	if desc != "" {
		base += "，" + desc
	}
	outfit := ""
	if desc == "" {
		outfit = "穿着简洁得体的日常服装"
	}
	label := pick(chibi, "Q版", "全尺寸") + pick(male, "男性", "女性") + "角色立绘"
	return PortraitPreset{Kind: pick(male, "通用-男", "通用-女"), Chibi: chibi, Label: label, Template: pick(chibi, templates["genericChibi"], templates["genericFull"]), Base: base, Outfit: outfit, Gender: pick(male, "男孩", "女孩"), DefaultRatio: defaultRatioValue}
}

func (s *session) handleGroupPhoto(args map[string]any) string {
	roles := normalizeGroupRoles(stringListArg(args, "roles"))
	if len(roles) == 0 {
		return "合照生成失败：roles 至少需要一位参与者"
	}
	chibi := boolArg(args, "chibi")
	profiles := mergeProfiles(s.prompts.Builtin, s.custom)
	planningProvided := strings.TrimSpace(stringArg(args, "planning")) != ""
	planning := strings.TrimSpace(stringArg(args, "planning"))
	if planning == "" {
		planning = defaultGroupPlanning(roles, chibi, profiles, s.prompts)
	}
	environment := strings.TrimSpace(stringArg(args, "environment"))
	if environment == "" {
		environment = s.prompts.randomGroupEnvironment()
	}
	members := make([]string, 0, len(roles))
	for _, item := range roles {
		members = append(members, groupMemberAppearance(profiles, item, chibi, !planningProvided))
	}
	proportion := ""
	if chibi {
		proportion = "Q版二头身，头身比约1:2，"
	}
	planningLine := ""
	if planning != "" {
		planningLine = "人物安排：" + planning + "。"
	}
	fullPrompt := s.prompts.renderTemplate(s.prompts.Templates["groupPhoto"], map[string]string{
		"proportion":  proportion,
		"count":       fmt.Sprintf("%d", len(roles)),
		"members":     strings.Join(members, "；"),
		"planning":    planningLine,
		"environment": environment,
	})
	useReference := s.reference.Image != ""
	plan := resolveDimensionPlan(stringArg(args, "ratio"), nil)
	if useReference {
		plan = s.planFor("")
	}
	params := GenerateImageParams{
		Prompt:          fullPrompt,
		NegativePrompt:  defaultString(stringArg(args, "negative_prompt"), s.prompts.Templates["defaultNegative"]),
		Width:           plan.Width,
		Height:          plan.Height,
		Steps:           normalizeSteps(numberArg(args, "steps")),
		CfgScale:        defaultFloat(numberArg(args, "cfg_scale"), 1.0),
		SuperResolution: boolArg(args, "super_resolution"),
	}
	if useReference {
		params.InitImg = s.reference.Image
		params.Strength = normalizeStrength(numberArg(args, "strength"), "合照")
	}
	return s.paint(params, "合照生成")
}

func (s *session) handleCreateCharacter(args map[string]any) string {
	name := strings.TrimSpace(stringArg(args, "name"))
	appearance := strings.TrimSpace(stringArg(args, "appearance"))
	if name == "" {
		return "创建角色失败：角色名不能为空"
	}
	if appearance == "" {
		return "创建角色失败：appearance 不能为空"
	}
	if name == "月华" || name == "琉璃" || name == "通用-女" || name == "通用-男" {
		return "创建角色失败：「" + name + "」是内置角色名，请换一个名字"
	}
	existed := false
	if _, ok := s.custom[name]; ok {
		existed = true
	}
	profile := CustomCharacterProfile{
		Name:        name,
		Gender:      defaultString(stringArg(args, "gender"), "女"),
		Appearance:  appearance,
		OutfitFull:  strings.TrimSpace(stringArg(args, "outfit_full")),
		OutfitChibi: strings.TrimSpace(stringArg(args, "outfit_chibi")),
	}
	if !saveCustomCharacter(profile) {
		return "创建角色失败：角色库写入失败，请稍后重试"
	}
	s.custom[name] = profile
	action := "已创建"
	if existed {
		action = "已更新"
	}
	return action + "自定义角色「" + name + "」（" + profile.Gender + "）。之后绘制立绘或合照时可直接用该名字引用。"
}

func (s *session) handleSwitchDiffusionScheme(args map[string]any) string {
	name := strings.TrimSpace(stringArg(args, "scheme"))
	if name == "" {
		return "切换模型方案失败：请指定 turbo 或 quality"
	}
	if err := config.SetActiveDiffusionScheme(name); err != nil {
		return "切换模型方案失败：" + err.Error()
	}
	label := name
	switch name {
	case "turbo":
		label = "速度优先"
	case "quality":
		label = "质量优先"
	}
	return "已切换到 " + name + "（" + label + "）模型方案，后续绘图将使用该方案。"
}

func (s *session) handleListImages() string {
	entries := listImages()
	if len(entries) == 0 {
		return "资源目录中还没有任何图片：当前没有可引用的图片句柄"
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		desc := "（暂无描述）"
		if entry.Summary != "" {
			desc = sanitizeImageDesc(entry.Summary, 40)
		}
		lost := ""
		if entry.State != "active" {
			lost = " [本体已回收]"
		}
		lines = append(lines, entry.Handle+" "+desc+lost)
	}
	return "资源目录中最近的图片（句柄越新越靠前）：\n" + strings.Join(lines, "\n") + "\n可调用 upscale_image 对指定图片超分；图生图默认沿用最近一次生成的画作，也可由对话者经 reference_tag 指定基座。" + s.referenceStatus()
}

func (s *session) handleUpscaleImage(args map[string]any) string {
	source := ""
	label := ""
	if tag := strings.TrimSpace(stringArg(args, "image_tag")); tag != "" {
		handle, resolvedSource, _, ok := resolveImageRef(tag)
		if !ok {
			return "超分失败：句柄不存在或已失效，请先调用 list_images 查看可用图片"
		}
		if resolvedSource == "" {
			return "超分失败：" + handle + " 的本体已从内容仓库回收，无法超分"
		}
		source = resolvedSource
		label = handle
	} else {
		if s.resultPath == "" {
			return "超分失败：还没有生成过作品，请通过 image_tag 指定图片句柄，或先完成一次图像生成"
		}
		source = s.resultPath
		label = "最近作品"
	}
	data, err := s.env.ReadImageBytes(source)
	if err != nil {
		return "超分失败: " + err.Error()
	}
	result, err := multimodal.UpscaleImage(data)
	if err != nil {
		return "超分失败: " + err.Error()
	}
	path, _ := result["path"].(string)
	width := toIntValue(result["width"])
	height := toIntValue(result["height"])
	if url := readURLOf(path); url != "" {
		if urls := s.env.PushImages([]string{url}); len(urls) > 0 {
			s.resultPath = urls[0]
			s.rememberRuntimeArtwork(urls[0], "最新超分作品")
		} else {
			s.rememberRuntimeArtwork(url, "最新超分作品")
		}
	}
	return fmt.Sprintf("超分成功。图片尺寸: %dx%d（%s），已推送给用户", width, height, label)
}

func collectPaintingDetail(call chatCall, paintings *[]PaintingDetail) {
	args := parseToolArgs(call.Arguments)
	name := call.Name
	switch name {
	case "portrait":
		character := defaultString(stringArg(args, "character"), "通用-女")
		*paintings = append(*paintings, PaintingDetail{
			ToolName: name, PromptSummary: characterLabel(character) + pick(boolArg(args, "chibi"), "Q版", "全尺寸") + "立绘",
			Character: character, Chibi: boolArg(args, "chibi"), Expression: stringArg(args, "expression"),
			Posture: stringArg(args, "posture"), Environment: stringArg(args, "environment"),
		})
	case "group_photo":
		roles := normalizeGroupRoles(stringListArg(args, "roles"))
		*paintings = append(*paintings, PaintingDetail{
			ToolName: name, PromptSummary: pick(boolArg(args, "chibi"), "Q版", "") + "与" + defaultString(strings.Join(roles, "、"), "他人") + "的合照",
			Roles: roles, Chibi: boolArg(args, "chibi"), Planning: stringArg(args, "planning"), Environment: stringArg(args, "environment"),
		})
	case "diffusion_generation", "image_to_image":
		prompt := stringArg(args, "prompt")
		strengthNote := ""
		if name == "image_to_image" {
			strengthNote = fmt.Sprintf("（强度 %.2f）", normalizeStrength(numberArg(args, "strength"), "图生图"))
		}
		*paintings = append(*paintings, PaintingDetail{ToolName: name, PromptSummary: truncateRunes(prompt, 100) + strengthNote})
	case "upscale_image":
		label := "最近作品超分重建"
		if tag := stringArg(args, "image_tag"); tag != "" {
			if handle, _, _, ok := resolveImageRef(tag); ok {
				label = handle + " 超分重建"
			}
		}
		*paintings = append(*paintings, PaintingDetail{ToolName: name, PromptSummary: label})
	}
}

func buildPaintingSummary(paintings []PaintingDetail) string {
	if len(paintings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(paintings)+1)
	for _, painting := range paintings {
		switch painting.ToolName {
		case "portrait":
			desc := ""
			switch painting.Character {
			case "月华":
				desc = "月华绘制了一幅" + pick(painting.Chibi, "Q版", "全尺寸") + "自画像"
			case "琉璃":
				desc = "月华为琉璃绘制了一幅" + pick(painting.Chibi, "Q版", "全尺寸") + "立绘"
			default:
				desc = "月华绘制了一幅" + pick(painting.Chibi, "Q版", "全尺寸") + characterLabel(painting.Character) + "立绘"
			}
			if painting.Expression != "" {
				desc += "，展现了" + painting.Expression
			}
			if painting.Environment != "" {
				desc += "，背景是" + painting.Environment
			}
			parts = append(parts, desc+"。")
		case "group_photo":
			desc := "月华绘制了一幅" + pick(painting.Chibi, "Q版", "") + "与" + defaultString(strings.Join(painting.Roles, "、"), "他人") + "的合照"
			if painting.Planning != "" {
				desc += "，" + painting.Planning
			}
			if painting.Environment != "" {
				desc += "，背景是" + painting.Environment
			}
			parts = append(parts, desc+"。")
		case "image_to_image":
			parts = append(parts, "月华参照参考图绘制了一幅图像："+painting.PromptSummary+"。")
		case "upscale_image":
			parts = append(parts, "月华为"+painting.PromptSummary+"提升了清晰度，图像已变得更精细。")
		default:
			parts = append(parts, "月华绘制了一幅图像："+painting.PromptSummary+"。")
		}
	}
	parts = append(parts, "图像已通过前端推送给用户。")
	return strings.Join(parts, "\n")
}

func normalizeGroupRoles(roles []string) []string {
	result := make([]string, 0, len(roles))
	seen := map[string]bool{}
	for _, role := range roles {
		value := strings.TrimSpace(role)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func characterLabel(character string) string {
	switch character {
	case "月华":
		return "月华"
	case "琉璃":
		return "琉璃"
	case "通用-男":
		return "男性角色"
	case "通用-女":
		return "女性角色"
	}
	if character == "" {
		return "角色"
	}
	return character
}

func groupMemberAppearance(profiles map[string]CharacterProfile, role string, chibi, withOutfit bool) string {
	profile, ok := profiles[role]
	if !ok {
		return role
	}
	outfit := ""
	if withOutfit {
		outfit = "，" + characterOutfit(profile, chibi)
	}
	return profile.Name + "，" + profile.Appearance + outfit
}

func defaultGroupPlanning(roles []string, chibi bool, profiles map[string]CharacterProfile, library *PromptLibrary) string {
	entries := make([]string, 0)
	for _, role := range roles {
		profile, ok := profiles[role]
		if !ok {
			continue
		}
		entries = append(entries, profile.Name+"："+library.randomExpression()+"，"+library.randomPosture()+"，"+characterOutfit(profile, chibi))
	}
	return strings.Join(entries, "；")
}
