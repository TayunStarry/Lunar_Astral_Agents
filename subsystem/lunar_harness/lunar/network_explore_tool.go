package lunar

// network_explore_tool.go 联网探索工具：聚合网页检索、哔哩观影、互助论坛与通讯桥接。
//
// 参数风格与 LTPX 聚合工具一致：只接收 tool（目标能力）与 instruction（自然语言任务）。
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

// networkExploreToolCalls 联网探索工具定义（tool + instruction，与 LTPX 参数风格一致）。
//
// 哔哩观影与互助论坛始终暴露：它们现在只登记生活事件，不再依赖智能体即时可用；
// qq_send 仍只在 QQ 桥接器连接成功时加入枚举。
func networkExploreToolCalls() []ToolCall {
	subTools := []string{networkWebSearchTool, networkBilibiliTool, networkMutualForumTool}
	summaryParts := []string{"web_search", "bilibili_viewer", "mutual_forum"}
	descriptionParts := []string{
		"web_search=网页检索",
		"bilibili_viewer=哔哩观影（安排下一轮去 B 站看视频，可用 instruction 引导选片方向）",
		"mutual_forum=互助论坛（安排下一轮去论坛逛逛，可用 instruction 引导话题与行动）",
	}
	if DialogueBridging.Ready() {
		subTools = append(subTools, networkQQSendTool)
		summaryParts = append(summaryParts, "qq_send")
		descriptionParts = append(descriptionParts, "qq_send=向指定 QQ 群/好友发送文本、图片或文件（图片/文件使用句柄）")
	}
	return []ToolCall{
		functionTool(networkExploreToolName,
			"联网探索：tool 选择 "+strings.Join(summaryParts, "、")+"，instruction 用自然语言说明任务；",
			map[string]any{
				"tool": map[string]any{
					"type":        "string",
					"enum":        subTools,
					"description": strings.Join(descriptionParts, "；"),
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "例如: 查一下今天的 AI 新闻；去 B 站看看有趣的猫咪视频；在互助论坛发帖问问 WebView 超时怎么排查；把这个文件 @D.012 发给群「测试群」",
				},
			}, []string{"tool", "instruction"}),
	}
}

// handleNetworkExplore 执行联网探索：按 tool 分派给对应智能体
func handleNetworkExplore(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ToolResult{Text: fmt.Sprintf("联网探索执行异常: %v", recovered)}
		}
	}()

	tool := strings.ToLower(strings.TrimSpace(stringArg(args, "tool")))
	instruction := strings.TrimSpace(stringArg(args, "instruction"))
	if instruction == "" {
		return ToolResult{Text: "请用自然语言说明这次联网探索要做什么"}
	}

	switch tool {
	case networkWebSearchTool:
		text, err := WebSearch.Run(instruction)
		return networkExploreResult("网页检索", text, err)
	case networkBilibiliTool:
		scheduleLifeEvent("watch", instruction)
		return ToolResult{Text: bilibiliFixedReply}
	case networkMutualForumTool:
		scheduleLifeEvent("forum", instruction)
		return ToolResult{Text: forumFixedReply}
	case networkQQSendTool:
		if !DialogueBridging.Ready() {
			return ToolResult{Text: "QQ 未就绪：暂时无法向 QQ 发送数据"}
		}
		text, err := DialogueBridging.Run(instruction)
		return networkExploreResult("通讯桥接", text, err)
	default:
		return ToolResult{Text: "tool 只能取 web_search / bilibili_viewer / mutual_forum / qq_send"}
	}
}

// scheduleLifeEvent 登记强制生活事件；调度器会在下一 Tick 跳过权重与其他体系直接执行它。
func scheduleLifeEvent(eventID, instruction string) {
	if !life.RequestEvent(eventID, instruction) {
		logger.Warn(lunarCoreModule, "[联网探索] 强制事件 %s 登记失败", eventID)
	}
}

// networkExploreResult 统一整理子智能体的结果与失败信息
func networkExploreResult(label, text string, err error) ToolResult {
	if err != nil {
		if strings.TrimSpace(text) != "" {
			return ToolResult{Text: fmt.Sprintf("【%s失败】%s\n已完成部分：\n%s", label, err.Error(), text)}
		}
		return ToolResult{Text: fmt.Sprintf("【%s失败】%s", label, err.Error())}
	}
	if strings.TrimSpace(text) == "" {
		text = label + "已完成"
	}
	return ToolResult{Text: text}
}
