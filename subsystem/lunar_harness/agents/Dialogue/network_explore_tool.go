package Dialogue

// network_explore_tool.go 联网探索工具：聚合网页检索、哔哩观影、互助论坛与通讯桥接。
//
// 输入输出严格使用统一原语：{type, instruction, handle, details} → {result, handle, error}。
// 网页检索与通讯桥接直接执行；哔哩观影与互助论坛已迁入生活链：
// 工具只登记强制事件，下一 Tick 由调度器跳过其他体系直接进入生活流程，
// instruction 会交给对应智能体引导关键词生成与运行流程。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/WebSearch"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/life"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// networkExploreToolName 联网探索聚合工具名
	networkExploreToolName = "network_explore"
	// networkWebSearchTool 网页检索子工具
	networkWebSearchTool = "web_search"
	// networkBilibiliTool 哔哩观影子工具（触发生活链观影事件）
	networkBilibiliTool = "bilibili_viewer"
	// networkMutualForumTool 互助论坛子工具（触发生活链论坛事件）
	networkMutualForumTool = "mutual_forum"
	// networkQQSendTool 通讯桥接子工具（仅在桥接器就绪时暴露）
	networkQQSendTool = "qq_send"

	// bilibiliFixedReply 哔哩观影触发后的固定响应
	bilibiliFixedReply = "月华等下就去B站逛逛"
	// forumFixedReply 互助论坛触发后的固定响应
	forumFixedReply = "月华等下就去论坛逛逛"
)

// networkExploreToolCalls 联网探索工具定义（统一原语：type + instruction + handle + details）。
//
// 哔哩观影与互助论坛始终暴露：它们现在只登记生活事件，不再依赖智能体即时可用；
// qq_send 仍只在 QQ 桥接器连接成功时加入枚举。
func networkExploreToolCalls() []ToolCall {
	subTools := []string{networkWebSearchTool, networkBilibiliTool, networkMutualForumTool}
	descriptionParts := []string{
		"bilibili_viewer=哔哩观影",
		"mutual_forum=互助论坛",
		"web_search=网页检索",
	}
	if DialogueBridging.Ready() {
		subTools = append(subTools, networkQQSendTool)
		descriptionParts = append(descriptionParts, "qq_send=向指定 QQ 群/好友发送文本、图片或文件（图片/文件使用句柄）")
	}
	return []ToolCall{
		schemaTool(networkExploreToolName,
			"使用自然语言去调度专用子智能体进行联网探索",
			agentdispatch.Schema(
				strings.Join(descriptionParts, "；"),
				subTools,
				"例如: 查一下今天的 AI 新闻；去 B 站看看有趣的猫咪视频；在互助论坛发帖问问 WebView 超时怎么排查；把这个文件 @D.012 发给群「测试群」",
				"qq_send 分支可传入待发送的图片 / 文件句柄（如 @I.007、@D.012）；其他分支按任务需要传入目标句柄。",
			)),
	}
}

// handleNetworkExplore 执行联网探索：按 type 分派给对应智能体。
// 输入输出严格使用统一原语，任何格式错误都返回标准 {result, handle, error}。
func handleNetworkExplore(args map[string]any) (result ToolResult) {
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
		if kind == networkQQSendTool && !DialogueBridging.Ready() {
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
	case networkWebSearchTool:
		text, runErr := WebSearch.Run(call.Instruction)
		return toolJSON(networkExploreResult("网页检索", text, runErr, nil))
	case networkBilibiliTool:
		scheduleLifeEvent("watch", call.Instruction)
		return toolJSON(agentdispatch.Success(bilibiliFixedReply, nil))
	case networkMutualForumTool:
		scheduleLifeEvent("forum", call.Instruction)
		return toolJSON(agentdispatch.Success(forumFixedReply, nil))
	case networkQQSendTool:
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
	if !life.RequestEvent(eventID, instruction) {
		logger.Warn(lunarCoreModule, "[联网探索] 强制事件 %s 登记失败", eventID)
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
