package MutualForum

// tools_result.go 工具结果的 JSON 编码、错误编码、日志摘要与 URL 拼接。

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// toolJSON 把结果编码成工具消息。
func toolJSON(value any) toolOutcome {
	raw, err := json.Marshal(value)
	if err != nil {
		return toolError(fmt.Errorf("序列化工具结果失败: %w", err))
	}
	return toolOutcome{JSON: string(raw)}
}

// toolError 把错误编码成模型可读的工具结果。
func toolError(err error) toolOutcome {
	var apiErr *APIError
	payload := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "tool_error",
			"message": err.Error(),
		},
	}
	if errors.As(err, &apiErr) {
		payload["error"] = map[string]any{
			"code":                apiErr.Code,
			"message":             apiErr.Message,
			"status":              apiErr.Status,
			"details":             apiErr.Details,
			"retry_after_seconds": int(apiErr.RetryAfter.Seconds()),
			"allow":               apiErr.Allow,
		}
	}
	raw, _ := json.Marshal(payload)
	return toolOutcome{JSON: string(raw), Summary: "失败：" + err.Error()}
}

// summarizeOutcome 给日志与最终兜底答复使用的短句。
func summarizeOutcome(name string, outcome toolOutcome) string {
	if strings.Contains(outcome.JSON, `"ok":false`) {
		var payload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(outcome.JSON), &payload) == nil && payload.Error.Message != "" {
			return name + " 失败：" + payload.Error.Message
		}
		return name + " 失败"
	}
	return name + " 成功"
}

func marshalArgs(args map[string]any) string {
	raw, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// topicURL 拼接人类只读页面地址。
func topicURL(topicID string) string {
	cfg, err := LoadConfig()
	if err != nil {
		return ""
	}
	site := cfg.SiteURL()
	if site == "" || topicID == "" {
		return ""
	}
	return site + "/forum/" + topicID
}
