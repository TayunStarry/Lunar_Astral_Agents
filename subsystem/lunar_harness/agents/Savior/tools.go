package Savior

// tools.go 拯救者的工具集：读上下文、改上下文、结束修复。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/agentdump"
	"LunarSubsystem/LunarHarness/engine/agentrepair"
)

// repairTools 返回修复工具定义。
func repairTools() []toolDefinition {
	return []toolDefinition{
		toolSchema(toolReadContext, "读取出错智能体的上下文快照；mode=recent 只看最近部分，mode=full 看当前全部", map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "目标智能体标识，缺省为本次故障的智能体"},
			"mode":     map[string]any{"type": "string", "enum": []string{"recent", "full"}, "description": "读取范围，默认 recent"},
			"limit":    map[string]any{"type": "integer", "description": "recent 模式下读取的消息条数，默认 8"},
		}),
		toolSchema(toolRepairContext, "对出错智能体的上下文执行一次修复动作，动作可用范围见系统提示词", map[string]any{
			"agent_id":  map[string]any{"type": "string", "description": "目标智能体标识，缺省为本次故障的智能体"},
			"action":    map[string]any{"type": "string", "description": "修复动作：trim / drop_images / clear / reset"},
			"keep_last": map[string]any{"type": "integer", "description": "trim 动作保留的消息条数，默认 8"},
		}, "action"),
		toolSchema(toolFinish, "结束本次修复并说明结论", map[string]any{
			"summary": map[string]any{"type": "string", "description": "修了什么、为什么这样修（2-3 句中文）"},
		}, "summary"),
	}
}

// toolSchema 组装一个函数工具声明。
func toolSchema(name, description string, properties map[string]any, required ...string) toolDefinition {
	var tool toolDefinition
	tool.Type = "function"
	tool.Function.Name = name
	tool.Function.Description = description
	tool.Function.Parameters = map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
	return tool
}

// executeTool 分发一次工具调用。
func executeTool(_ context.Context, failure *Failure, name, rawArgs string) toolOutcome {
	args := parseArgs(rawArgs)
	agentID := stringArg(args, "agent_id")
	if agentID == "" {
		agentID = failure.Agent
	}
	switch name {
	case toolReadContext:
		return readContextOutcome(agentID, args)
	case toolRepairContext:
		return repairContextOutcome(agentID, args)
	case toolFinish:
		summary := strings.TrimSpace(stringArg(args, "summary"))
		if summary == "" {
			summary = "已结束修复"
		}
		return toolOutcome{JSON: jsonText(map[string]any{"ok": true, "summary": summary}), Summary: summary, Done: true}
	default:
		return toolOutcome{
			JSON:    jsonText(map[string]any{"ok": false, "error": "未知工具：" + name}),
			Summary: "未知工具：" + name,
		}
	}
}

// readContextOutcome 读取上下文：优先注册表，其次退回上下文导出。
func readContextOutcome(agentID string, args map[string]any) toolOutcome {
	mode := strings.ToLower(strings.TrimSpace(stringArg(args, "mode")))
	limit := intArg(args, "limit")
	if mode != "full" {
		mode = "recent"
		if limit <= 0 {
			limit = recentMessageLimit
		}
	} else {
		limit = 0
	}
	text, err := agentrepair.Inspect(agentID, limit)
	if err != nil {
		text, err = agentdump.Export(agentID, mode == "full")
	}
	if err != nil {
		return toolOutcome{
			JSON:    jsonText(map[string]any{"ok": false, "error": err.Error()}),
			Summary: "读取上下文失败",
		}
	}
	text = previewRunes(text, maxInspectRunes)
	return toolOutcome{
		JSON:    jsonText(map[string]any{"ok": true, "agent_id": agentID, "mode": mode, "context": text}),
		Summary: fmt.Sprintf("已读取 %s 的%s上下文（%d 字）", agentID, mode, len([]rune(text))),
	}
}

// repairContextOutcome 执行一次修复动作。
func repairContextOutcome(agentID string, args map[string]any) toolOutcome {
	action := strings.TrimSpace(stringArg(args, "action"))
	params := map[string]any{}
	if keep := intArg(args, "keep_last"); keep > 0 {
		params["keep_last"] = keep
	}
	summary, err := agentrepair.Repair(agentID, action, params)
	if err != nil {
		return toolOutcome{
			JSON:    jsonText(map[string]any{"ok": false, "error": err.Error()}),
			Summary: "修复动作失败：" + err.Error(),
		}
	}
	return toolOutcome{
		JSON:    jsonText(map[string]any{"ok": true, "agent_id": agentID, "action": action, "summary": summary}),
		Summary: summary,
	}
}

// parseArgs 解析工具参数；解析失败按空参数处理。
func parseArgs(raw string) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return args
	}
	_ = json.Unmarshal([]byte(raw), &args)
	return args
}

// stringArg 读取字符串参数。
func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

// intArg 读取整数参数（兼容 JSON 数字解析出的 float64）。
func intArg(args map[string]any, key string) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case json.Number:
		number, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(number)
	default:
		return 0
	}
}

// jsonText 把结果序列化为工具消息内容。
func jsonText(payload map[string]any) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return `{"ok":false,"error":"结果序列化失败"}`
	}
	return string(raw)
}
