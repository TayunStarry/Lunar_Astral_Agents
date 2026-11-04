package thinking

// tool_registry.go 工具注册表装配层：把智能体声明的 schema 与链侧执行器接起来。
//
// 声明：agents/Dialogue 在 init() 中注册（dispatch_technique / fetch_resources /
// life_planning / network_explore）；
// 实现：本文件在装配时把链侧处理器注册进 toolreg，链只通过注册表查表执行；
// LTPX 扩展应用工具是运行时聚合，仍由链侧按运行时清单追加。

import (
	"context"
	"fmt"
	"sync"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

var registerDialogueToolsOnce sync.Once

// registerDialogueTools 注册对话者工具的执行器（装配层职责）。
func registerDialogueTools() {
	registerDialogueToolsOnce.Do(func() {
		registerTool(Dialogue.ToolDispatchTechnique, handleDispatchTechnique)
		registerTool(Dialogue.ToolFetchResources, handleFetchResources)
		registerTool(Dialogue.ToolLifePlanning, handleLifePlanning)
		registerTool(Dialogue.ToolNetworkExplore, handleNetworkExplore)
		registerTool(crystalToolName, handleUseTheProgram)
		logger.Info(lunarCoreModule, "[工具] 对话者工具执行器已注册")
	})
}

// registerTool 把链侧处理器适配成 toolreg 执行器。
func registerTool(name string, handler ToolHandler) {
	toolreg.RegisterExecutor(name, func(_ context.Context, args map[string]any) (any, error) {
		return handler(args), nil
	})
}

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

// dialogueToolCalls 从注册表派生对话者的模型工具清单；LTPX 运行时工具单独追加。
func dialogueToolCalls() []ToolCall {
	schemas := toolreg.Schemas("dialogue")
	calls := make([]ToolCall, 0, len(schemas)+1)
	for _, schema := range schemas {
		calls = append(calls, registrySchemaToCall(schema))
	}
	calls = append(calls, getCrystalRemoteTools()...)
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
