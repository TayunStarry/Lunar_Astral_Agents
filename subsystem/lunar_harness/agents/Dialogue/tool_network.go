package Dialogue

// tool_network.go 联网探索工具执行器：聚合网页检索与通讯桥接。
//
// 输入输出严格使用统一原语：{type, instruction, handle, details} → {result, handle, error}。
// 观影与互助论坛已迁移到 life_planning：由它登记强制生活事件，下一 Tick 直接进入生活链路。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// NetworkWebSearchTool 网页检索子工具
	NetworkWebSearchTool = "web_search"
	// NetworkQQSendTool 通讯桥接子工具（仅在桥接器就绪时暴露）
	NetworkQQSendTool = "qq_send"

	// ForumFixedReply 互助论坛触发后的固定响应
	ForumFixedReply = "月华等下就去论坛逛逛"
	// WatchFixedReply 看视频触发后的固定响应
	WatchFixedReply = "月华这就去看会儿视频"
)

// ExecuteNetworkExplore 执行联网探索：按 type 分派给对应智能体。
// 输入输出严格使用统一原语，任何格式错误都返回标准 {result, handle, error}。
func ExecuteNetworkExplore(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = toolJSON(agentdispatch.Failure(fmt.Sprintf("联网探索执行异常: %v", recovered), nil))
		}
	}()

	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	if call.Details {
		if kind == NetworkQQSendTool && !DialogueBridging.Ready() {
			return toolJSON(agentdispatch.Failure("type 只能取 web_search", nil))
		}
		if text, ok := networkScope(kind); ok {
			return toolJSON(agentdispatch.Success(text, nil))
		}
		return toolJSON(agentdispatch.Failure("type 只能取 web_search / qq_send", nil))
	}
	if call.Instruction == "" {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明这次联网探索要做什么", nil))
	}

	switch kind {
	case NetworkWebSearchTool:
		if WebSearch == nil {
			return toolJSON(agentdispatch.Failure("【网页检索失败】检索链未接入", nil))
		}
		text, runErr := WebSearch(call.Instruction)
		return toolJSON(networkExploreResult("网页检索", text, runErr, nil))
	case NetworkQQSendTool:
		if !DialogueBridging.Ready() {
			return toolJSON(agentdispatch.Failure("QQ 未就绪：暂时无法向 QQ 发送数据", nil))
		}
		instruction := agentdispatch.WithHandles(call.Instruction, call.Handle)
		text, runErr := DialogueBridging.Run(instruction)
		return toolJSON(networkExploreResult(agentinfo.NameDialogueBridging, text, runErr, call.Handle))
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 web_search / qq_send", nil))
	}
}

// networkExploreResult 统一整理子智能体的结果与失败信息
func networkExploreResult(label, text string, err error, handles []string) agentdispatch.Result {
	if err != nil {
		if strings.TrimSpace(text) != "" {
			return agentdispatch.PartialFailure(fmt.Sprintf("【%s失败】%s", label, err.Error()), text, handles)
		}
		return agentdispatch.Failure(fmt.Sprintf("【%s失败】%s", label, err.Error()), handles)
	}
	if strings.TrimSpace(text) == "" {
		text = label + "已完成"
	}
	return agentdispatch.Success(text, handles)
}

// networkScope 返回联网探索各分支的职能范围描述。
func networkScope(kind string) (string, bool) {
	switch kind {
	case NetworkWebSearchTool:
		return "网页检索子智能体：联网搜索并用自然语言返回摘要与来源。", true
	case NetworkQQSendTool:
		return "通讯桥接子智能体：向指定 QQ 群或好友发送文本、图片或文件；handle 传待发送的图片 / 文件句柄。", true
	}
	return "", false
}
