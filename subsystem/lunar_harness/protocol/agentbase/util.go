package agentbase

// util.go 消息内容与文本截断的公共工具。

import (
	"encoding/json"
	"strings"

	"LunarSubsystem/LunarHarness/internal/textutil"
)

// ParseToolArgs 解析模型给出的 JSON 参数；空串视为空对象。
func ParseToolArgs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return map[string]any{}, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

// StringOf 把消息 content（字符串 / 片段数组）还原为纯文本
func StringOf(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			if part, ok := item.(map[string]any); ok {
				if text, ok := part["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// TruncateRunes 按字符截断（超长时以省略号收尾）。
// 实现唯一收敛在 internal/textutil，此处仅保留包级入口供既有调用方使用。
func TruncateRunes(text string, limit int) string { return textutil.TruncateRunes(text, limit) }

// HardCutRunes 按字符硬切断（超长时以省略号收尾）。
// 与 textutil.HardCutRunes（带「已截断」标记）语义不同，唯一实现在 internal/textutil.CutRunes。
func HardCutRunes(text string, limit int) string { return textutil.CutRunes(text, limit) }
