package lunar

import (
	"strings"
)

// newPainterPromptLibrary 读取并构建绘制者提示词库
func newPainterPromptLibrary() *PainterPromptLibrary {
	common := loadPromptDocument("prompts/painterCommon.md")
	characters := loadPromptDocument("prompts/painterCharacters.md")
	library := &PainterPromptLibrary{
		Common:     common,
		Characters: characters,
		Templates:  map[string]string{},
		Builtin:    map[string]CharacterProfile{},
	}
	library.Templates["chibiSelf"] = characters.text("月华/立绘-Q版")
	library.Templates["fullSelf"] = characters.text("月华/立绘-全尺寸")
	library.Templates["crystalFull"] = characters.text("琉璃/立绘-全尺寸")
	library.Templates["crystalChibi"] = characters.text("琉璃/立绘-Q版")
	library.Templates["genericFull"] = characters.text("通用角色/立绘-全尺寸")
	library.Templates["genericChibi"] = characters.text("通用角色/立绘-Q版")
	library.Templates["femaleBase"] = characters.text("通用角色/底板-女")
	library.Templates["maleBase"] = characters.text("通用角色/底板-男")
	library.Templates["defaultOutfitChibi"] = characters.text("月华/默认服装-Q版")
	library.Templates["defaultOutfitFull"] = characters.text("月华/默认服装-全尺寸")
	library.Templates["crystalOutfitFull"] = characters.text("琉璃/默认服装-全尺寸")
	library.Templates["crystalOutfitChibi"] = characters.text("琉璃/默认服装-Q版")
	library.Templates["defaultNegative"] = common.text("默认负面提示词")
	library.Templates["groupPhoto"] = common.text("合照模板")
	library.Builtin["月华"] = CharacterProfile{
		CustomCharacterProfile: CustomCharacterProfile{
			Name:        "月华",
			Gender:      "女",
			Appearance:  characters.text("月华/基础外观"),
			OutfitFull:  library.Templates["defaultOutfitFull"],
			OutfitChibi: library.Templates["defaultOutfitChibi"],
		},
		Builtin: true,
	}
	library.Builtin["琉璃"] = CharacterProfile{
		CustomCharacterProfile: CustomCharacterProfile{
			Name:        "琉璃",
			Gender:      "女",
			Appearance:  characters.text("琉璃/基础外观"),
			OutfitFull:  library.Templates["crystalOutfitFull"],
			OutfitChibi: library.Templates["crystalOutfitChibi"],
		},
		Builtin: true,
	}
	return library
}

// renderTemplate 按 {key} 占位符填充模板
func (library *PainterPromptLibrary) renderTemplate(template string, vars map[string]string) string {
	filled := template
	for key, value := range vars {
		filled = strings.ReplaceAll(filled, "{"+key+"}", value)
	}
	return tidyPrompt(filled)
}

func tidyPrompt(text string) string {
	text = regexpCommaComma.ReplaceAllString(text, "，")
	text = regexpCommaPeriod.ReplaceAllString(text, "。")
	text = regexpPeriodPeriod.ReplaceAllString(text, "。")
	text = regexpSpaceRun.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// renderAppearance 将基础外观、表情、服装、姿势、环境写入外观模板
func (library *PainterPromptLibrary) renderAppearance(template string, parts AppearanceParts, defaultOutfit, base, gender string) string {
	expression := parts.Expression
	if expression == "" {
		expression = library.randomExpression()
	}
	posture := parts.Posture
	if posture == "" {
		posture = library.randomPosture()
	}
	outfit := parts.Outfit
	if outfit == "" {
		outfit = defaultOutfit
	}
	if base == "" {
		base = library.Builtin["月华"].Appearance
	}
	if gender == "" {
		gender = "女孩"
	}
	return library.renderTemplate(template, map[string]string{
		"base":        base,
		"gender":      gender,
		"expression":  expression,
		"outfit":      outfit,
		"posture":     posture,
		"environment": parts.Environment,
	})
}

func (library *PainterPromptLibrary) randomExpression() string {
	items := library.Common.list("默认表情")
	return items[randomFloor(0, len(items)-1)]
}

func (library *PainterPromptLibrary) randomPosture() string {
	items := library.Common.list("默认姿势")
	return items[randomFloor(0, len(items)-1)]
}

func (library *PainterPromptLibrary) randomGroupEnvironment() string {
	items := library.Common.list("合照背景")
	return items[randomFloor(0, len(items)-1)]
}

// characterOutfit 角色默认服装
func characterOutfit(profile CharacterProfile, chibi bool) string {
	outfit := profile.OutfitFull
	if chibi {
		outfit = profile.OutfitChibi
		if outfit == "" {
			outfit = profile.OutfitFull
		}
	}
	if outfit == "" {
		return "穿着简洁得体的日常服装"
	}
	return outfit
}

// mergeProfiles 合并内置角色档案与自定义角色
func mergeProfiles(builtin map[string]CharacterProfile, custom map[string]CustomCharacterProfile) map[string]CharacterProfile {
	return mergeProfilesWithBuiltin(builtin, custom)
}

// mergeProfilesWithBuiltin 合并内置角色档案与自定义角色
func mergeProfilesWithBuiltin(builtin map[string]CharacterProfile, custom map[string]CustomCharacterProfile) map[string]CharacterProfile {
	profiles := make(map[string]CharacterProfile, len(builtin)+len(custom))
	for name, profile := range builtin {
		profiles[name] = profile
	}
	for name, profile := range custom {
		profiles[name] = CharacterProfile{CustomCharacterProfile: profile, Builtin: false}
	}
	return profiles
}
