package Dialogue

// prompts.go 对话者系统提示词的组装（统一放在 LunarHarness/prompts）。

import (
	"regexp"
	"sort"
	"strings"

	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/prompts"
)

var friendsSectionRegex = regexp.MustCompile(`\n?\{friends-appearance\}\n?`)

// SystemPrompt 组装对话者系统提示词（注入双方样貌与朋友样貌）。
func SystemPrompt(lunarAppearance, crystalAppearance string) string {
	raw, err := prompts.Read("prompts/dialogueRole.md")
	if err != nil {
		logger.Error(LoggerModule, "读取"+Role+"提示词失败: %v", err)
		return "你的名字叫做月华, 是一个女孩子"
	}
	prompt := strings.ReplaceAll(string(raw), "{lunar-appearance}", lunarAppearance)
	prompt = strings.ReplaceAll(prompt, "{crystal-appearance}", crystalAppearance)
	return applyFriendsAppearance(prompt)
}

// ReadAppendSettings 读取并规整追加设定。
func ReadAppendSettings() []string {
	settings := make([]string, 0, len(config.AgentAppendSettings))
	for _, line := range config.AgentAppendSettings {
		text := strings.TrimSpace(strings.ReplaceAll(line, "\r\n", "\n"))
		if text != "" {
			settings = append(settings, text)
		}
	}
	return settings
}

// buildFriendsAppearanceSection 构建「朋友样貌」段落
func buildFriendsAppearanceSection() string {
	characters := Painter.CustomCharacters()
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
		profile := characters[name]
		if strings.TrimSpace(profile.Appearance) == "" {
			continue
		}
		lines = append(lines, "- "+name+"："+profile.Look(false))
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
