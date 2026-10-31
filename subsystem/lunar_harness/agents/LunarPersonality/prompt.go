package LunarPersonality

import (
	"regexp"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

var friendsSectionRegex = regexp.MustCompile(`\n?\{friends-appearance\}\n?`)

// composeDialoguePrompt 组装对话者系统提示词
func composeDialoguePrompt(lunarAppearance, crystalAppearance string) string {
	raw, err := prompts.Read("prompts/dialogueRole.md")
	if err != nil {
		logger.Error(lunarCoreModule, "读取对话者提示词失败: %v", err)
		return "你的名字叫做月华, 是一个女孩子"
	}
	prompt := strings.ReplaceAll(string(raw), "{lunar-appearance}", lunarAppearance)
	prompt = strings.ReplaceAll(prompt, "{crystal-appearance}", crystalAppearance)
	return applyFriendsAppearance(prompt)
}

// buildFriendsAppearanceSection 构建「朋友样貌」段落
func buildFriendsAppearanceSection() string {
	characters := loadCustomCharacters()
	names := make([]string, 0, len(characters))
	for name := range characters {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0)
	for _, name := range names {
		if name == "月华" || name == "琉璃" {
			continue
		}
		appearance := strings.TrimSpace(characters[name].Appearance)
		if appearance == "" {
			continue
		}
		lines = append(lines, "- "+name+"："+appearance)
	}
	if len(lines) == 0 {
		return ""
	}
	return "## 朋友样貌\n\n" + strings.Join(lines, "\n") + "\n"
}

// applyFriendsAppearance 注入朋友样貌；无自定义角色时连标题一起移除
func applyFriendsAppearance(prompt string) string {
	section := buildFriendsAppearanceSection()
	if section == "" {
		return friendsSectionRegex.ReplaceAllString(prompt, "\n")
	}
	return strings.Replace(prompt, "{friends-appearance}", section, 1)
}

// readAppendSettings 读取并规整追加设定
func readAppendSettings() []string {
	settings := make([]string, 0, len(config.AgentAppendSettings))
	for _, line := range config.AgentAppendSettings {
		text := strings.TrimSpace(strings.ReplaceAll(line, "\r\n", "\n"))
		if text != "" {
			settings = append(settings, text)
		}
	}
	return settings
}

// composePainterPrompt 组装绘制者系统提示词
func composePainterPrompt(lunarAppearance, crystalAppearance string) string {
	raw, err := prompts.Read("prompts/painterRole.md")
	if err != nil {
		return ""
	}
	prompt := strings.ReplaceAll(string(raw), "{lunar-appearance}", lunarAppearance)
	return strings.ReplaceAll(prompt, "{crystal-appearance}", crystalAppearance)
}

// readPrompt 读取内嵌提示词（失败返回空串）
func readPrompt(name string) string {
	raw, err := prompts.Read(name)
	if err != nil {
		logger.Error(lunarCoreModule, "读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return string(raw)
}
