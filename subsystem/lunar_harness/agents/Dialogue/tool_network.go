package Dialogue

// tool_network.go 联网探索工具执行器：聚合网页检索、哔哩观影、互助论坛与通讯桥接。
//
// 输入输出严格使用统一原语：{type, instruction, handle, details} → {result, handle, error}。
// 网页检索与通讯桥接直接执行；哔哩观影与互助论坛属生活链职责：
// 工具只登记强制事件，下一 Tick 由调度器跳过其他体系直接进入生活流程。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// NetworkWebSearchTool 网页检索子工具
	NetworkWebSearchTool = "web_search"
	// NetworkBilibiliTool 哔哩观影子工具（触发生活链看视频事件）
	NetworkBilibiliTool = "bilibili_viewer"
	// NetworkMutualForumTool 互助论坛子工具（触发生活链论坛事件）
	NetworkMutualForumTool = "mutual_forum"
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
			return toolJSON(agentdispatch.Failure("type 只能取 web_search / bilibili_viewer / mutual_forum", nil))
		}
		if text, ok := networkScope(kind); ok {
			return toolJSON(agentdispatch.Success(text, nil))
		}
		return toolJSON(agentdispatch.Failure("type 只能取 web_search / bilibili_viewer / mutual_forum / qq_send", nil))
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
	case NetworkBilibiliTool:
		// 与生活链权重分支共用同一条看视频流程：登记强制事件，下一 Tick 由生活链执行。
		scheduleLifeEvent("watch", call.Instruction)
		return toolJSON(agentdispatch.Success(WatchFixedReply, nil))
	case NetworkMutualForumTool:
		scheduleLifeEvent("forum", call.Instruction)
		return toolJSON(agentdispatch.Success(ForumFixedReply, nil))
	case NetworkQQSendTool:
		if !DialogueBridging.Ready() {
			return toolJSON(agentdispatch.Failure("QQ 未就绪：暂时无法向 QQ 发送数据", nil))
		}
		instruction := agentdispatch.WithHandles(call.Instruction, call.Handle)
		text, runErr := DialogueBridging.Run(instruction)
		return toolJSON(networkExploreResult("通讯桥接", text, runErr, call.Handle))
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 web_search / bilibili_viewer / mutual_forum / qq_send", nil))
	}
}

// scheduleLifeEvent 登记强制生活事件；调度器会在下一 Tick 跳过权重与其他体系直接执行它。
func scheduleLifeEvent(eventID, instruction string) {
	if RequestLifeEvent == nil {
		logger.Warn(LoggerModule, "[联网探索] 生活链未接入，强制事件 %s 登记失败", eventID)
		return
	}
	if !RequestLifeEvent(eventID, instruction) {
		logger.Warn(LoggerModule, "[联网探索] 强制事件 %s 登记失败", eventID)
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
	case NetworkBilibiliTool:
		return "哔哩观影子智能体：登记看视频事件，由生活链播放一个待看片段、理解画面并在原视频评论区留下观感。", true
	case NetworkMutualForumTool:
		return "互助论坛子智能体：登记论坛事件，由生活链进入互助论坛流程。", true
	case NetworkQQSendTool:
		return "通讯桥接子智能体：向指定 QQ 群或好友发送文本、图片或文件；handle 传待发送的图片 / 文件句柄。", true
	}
	return "", false
}
