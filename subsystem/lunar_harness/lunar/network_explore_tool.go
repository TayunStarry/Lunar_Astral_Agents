package lunar

// network_explore_tool.go 联网探索工具：聚合网页检索、互助论坛与通讯桥接。
//
// 参数风格与 LTPX 聚合工具一致：只接收 tool（目标能力）与 instruction（自然语言任务）。
// 具体流程、是否分享、看几个视频等编排由对应智能体自行决定，对话者只表达需求。
//
// 哔哩观影不再作为对话者可直接调用的子工具：它已迁入生活体系，由生活事件自主触发。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/MutualForum"
	"LunarSubsystem/LunarHarness/agents/WebSearch"
)

const (
	// networkExploreToolName 联网探索聚合工具名
	networkExploreToolName = "network_explore"
	// networkWebSearchTool 网页检索子工具
	networkWebSearchTool = "web_search"
	// networkQQSendTool 通讯桥接子工具（仅在桥接器就绪时暴露）
	networkQQSendTool = "qq_send"
	// networkMutualForumTool Mutual Forum 论坛子工具（仅在论坛可达且配置开启时暴露）
	networkMutualForumTool = "mutual_forum"
)

// networkExploreToolCalls 联网探索工具定义（tool + instruction，与 LTPX 参数风格一致）。
//
// qq_send 只在 QQ 桥接器连接成功、且能解析出群/好友信息时加入枚举；
// mutual_forum 只在独立配置开启、地址有效且论坛健康检查通过时加入枚举。
// 未就绪时模型根本看不到这个能力，避免它把需求发给一个不存在的通道。
func networkExploreToolCalls() []ToolCall {
	subTools := []string{networkWebSearchTool}
	summaryParts := []string{"web_search"}
	descriptionParts := []string{"web_search=网页检索"}
	if MutualForum.Ready() {
		subTools = append(subTools, networkMutualForumTool)
		summaryParts = append(summaryParts, "mutual_forum")
		descriptionParts = append(descriptionParts, "mutual_forum=互助论坛（查话题/搜索/发帖求助/回帖/管理自己发起的话题）")
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
					"description": "例如: 查一下今天的 AI 新闻；去 B 站看看有趣的游戏视频；在互助论坛发帖问问 WebView 超时怎么排查；把这个文件 @D.012 发给群「测试群」",
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
	case networkMutualForumTool:
		text, err := MutualForum.Run(instruction)
		return networkExploreResult("互助论坛", text, err)
	case networkQQSendTool:
		if !DialogueBridging.Ready() {
			return ToolResult{Text: "QQ 未就绪：暂时无法向 QQ 发送数据"}
		}
		text, err := DialogueBridging.Run(instruction)
		return networkExploreResult("通讯桥接", text, err)
	default:
		return ToolResult{Text: "tool 只能取 web_search / mutual_forum / qq_send"}
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
