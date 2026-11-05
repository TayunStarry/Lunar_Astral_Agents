package thinking

// tool_registry.go 工具注册表查询面：声明与执行器都由智能体包注册进 toolreg，
// 思维链只通过注册表查表派生模型工具清单并执行工具；链侧不再持有工具实现。

import (
	"context"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

// lookupTool 从注册表查找执行器并还原为链侧处理器。
func lookupTool(name string) ToolHandler {
	exec, ok := toolreg.LookupExecutor(name)
	if !ok {
		return nil
	}
	return func(args map[string]any) ToolResult {
		out, err := exec(context.Background(), args)
		if err != nil {
			return ToolResult{Text: "工具执行失败: " + err.Error()}
		}
		if result, ok := out.(ToolResult); ok {
			return result
		}
		return ToolResult{Text: fmt.Sprint(out)}
	}
}

// dialogueToolCalls 从注册表派生对话者的模型工具清单（含运行时动态枚举的扩展应用）。
func dialogueToolCalls() []ToolCall {
	schemas := toolreg.Schemas("dialogue")
	calls := make([]ToolCall, 0, len(schemas))
	for _, schema := range schemas {
		calls = append(calls, registrySchemaToCall(schema))
	}
	return calls
}

// networkExploreToolCalls 兼容旧调用：按名称从注册表取联网探索工具声明。
func networkExploreToolCalls() []ToolCall {
	return registryNamedToolCalls(Dialogue.ToolNetworkExplore)
}

// fetchResourceToolCalls 兼容旧调用：按名称从注册表取统一资源工具声明。
func fetchResourceToolCalls() []ToolCall {
	return registryNamedToolCalls(Dialogue.ToolFetchResources)
}

// registryNamedToolCalls 按名称返回单条工具声明。
func registryNamedToolCalls(name string) []ToolCall {
	schema, ok := toolreg.LookupSchema(name)
	if !ok {
		return nil
	}
	return []ToolCall{registrySchemaToCall(schema)}
}

// registrySchemaToCall 把注册表声明转成模型侧的 function calling 定义。
func registrySchemaToCall(schema toolreg.Schema) ToolCall {
	return ToolCall{Type: "function", Function: ToolDefinition{
		Name:        schema.Name,
		Description: schema.Description,
		Parameters:  schema.Parameters,
	}}
}
