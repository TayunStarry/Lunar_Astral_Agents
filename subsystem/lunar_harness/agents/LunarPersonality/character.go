package LunarPersonality

import (
	"encoding/json"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// loadCustomCharacters 读取全部自定义角色
func loadCustomCharacters() map[string]CustomCharacterProfile {
	characters := make(map[string]CustomCharacterProfile)
	entries, err := loadKnowledgeEntries("painter_characters")
	if err != nil {
		logger.Warn(lunarCoreModule, "[角色库] 读取失败: %v", err)
		return characters
	}
	for _, pair := range entries {
		if len(pair) < 2 || pair[0] == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(pair[1]), &raw) != nil {
			continue
		}
		if profile := normalizeProfile(raw, pair[0]); profile != nil {
			characters[profile.Name] = *profile
		}
	}
	return characters
}

// saveCustomCharacter 创建或更新自定义角色
func saveCustomCharacter(profile CustomCharacterProfile) bool {
	characters := loadCustomCharacters()
	characters[profile.Name] = profile
	entries := make([][]string, 0, len(characters))
	for name, item := range characters {
		raw, err := json.Marshal(item)
		if err != nil {
			continue
		}
		entries = append(entries, []string{name, string(raw)})
	}
	return saveKnowledgeEntries("painter_characters", entries)
}

// normalizeProfile 规整角色档案
func normalizeProfile(raw map[string]any, fallbackName string) *CustomCharacterProfile {
	name := strings.TrimSpace(stringArg(raw, "name"))
	if name == "" {
		name = fallbackName
	}
	appearance := strings.TrimSpace(stringArg(raw, "appearance"))
	if name == "" || appearance == "" {
		return nil
	}
	gender := "女"
	if stringArg(raw, "gender") == "男" {
		gender = "男"
	}
	return &CustomCharacterProfile{
		Name:        name,
		Gender:      gender,
		Appearance:  appearance,
		OutfitFull:  strings.TrimSpace(stringArg(raw, "outfitFull")),
		OutfitChibi: strings.TrimSpace(stringArg(raw, "outfitChibi")),
	}
}
