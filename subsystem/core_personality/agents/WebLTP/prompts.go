package WebLTP

// prompts.go 网络搜索的提示词加载（统一放在 CorePersonality/prompts）
//
// 任务型提示词用 `---user---` 分隔系统段与用户模板：
// 系统段是稳定的行为约束，用户模板只保留带 {占位符} 的数据骨架。

import (
	"regexp"
	"strings"

	"CorePersonality/prompts"
	"LunarSubsystem/LoggerGeneral"
)

// promptUserMarker 任务型提示词的「系统段 / 用户模板」分隔符
const promptUserMarker = "\n---user---\n"

// placeholderPattern 提示词占位符形态（{key}）
var placeholderPattern = regexp.MustCompile(`\{[A-Za-z0-9_\-]+\}`)

// loadPrompt 读取提示词文本（读取失败返回空串）
func loadPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		LoggerGeneral.Error("CrystalAstral", "网络搜索读取提示词 %s 失败: %v", name, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

// loadPromptPair 读取任务型提示词的系统段与用户模板
func loadPromptPair(name string) (string, string) {
	raw := loadPrompt(name)
	if raw == "" {
		return "", ""
	}
	if index := strings.Index(raw, promptUserMarker); index >= 0 {
		return strings.TrimSpace(raw[:index]), strings.TrimSpace(raw[index+len(promptUserMarker):])
	}
	return raw, ""
}

// renderPrompt 单次替换提示词中的 {占位符}（pairs 为「占位符, 值」成对传入）
//
// 单次替换避免正文里的 `{xxx}` 被二次展开（网页正文含花括号很常见）。
func renderPrompt(template string, pairs ...string) string {
	values := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return placeholderPattern.ReplaceAllStringFunc(template, func(key string) string {
		if value, ok := values[key]; ok {
			return value
		}
		return key
	})
}
