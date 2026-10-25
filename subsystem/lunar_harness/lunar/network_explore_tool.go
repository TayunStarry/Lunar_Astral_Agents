package lunar

// network_explore_tool.go 联网探索工具：聚合网页检索与哔哩观影。
//
// 参数风格与 LTPX 聚合工具一致：只接收 tool（目标能力）与 instruction（自然语言任务）。
// 具体流程、是否分享、看几个视频等编排由对应智能体自行决定，对话者只表达需求。

import (
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
	"LunarSubsystem/LunarHarness/agents/QQMessenger"
	"LunarSubsystem/LunarHarness/agents/WebSearch"
)

const (
	// networkExploreToolName 联网探索聚合工具名
	networkExploreToolName = "network_explore"
	// networkWebSearchTool 网页检索子工具
	networkWebSearchTool = "web_search"
	// networkBilibilViewerTool 哔哩观影子工具
	networkBilibilViewerTool = "bilibil_viewer"
	// networkQQSendTool QQ 投递子工具（仅在桥接器就绪时暴露）
	networkQQSendTool = "qq_send"
)

// networkExploreToolCalls 联网探索工具定义（tool + instruction，与 LTPX 参数风格一致）。
//
// qq_send 只在 QQ 桥接器连接成功、且能解析出群/好友信息时加入枚举：
// 未就绪时模型根本看不到这个能力，避免它把需求发给一个不存在的通道。
func networkExploreToolCalls() []ToolCall {
	subTools := []string{networkWebSearchTool, networkBilibilViewerTool}
	summary := "web_search（网页检索）或 bilibil_viewer（哔哩观影）"
	description := "web_search=网页检索；bilibil_viewer=哔哩观影（刷视频、评论、日记动态、分享）"
	if QQMessenger.Ready() {
		subTools = append(subTools, networkQQSendTool)
		summary = "web_search（网页检索）、bilibil_viewer（哔哩观影）或 qq_send（QQ 投递）"
		description += "；qq_send=向指定 QQ 群/好友发送文本、图片或文件（图片/文件使用句柄）"
	}
	return []ToolCall{
		functionTool(networkExploreToolName,
			"联网探索：tool 选择 "+summary+"，instruction 用自然语言说明任务；",
			map[string]any{
				"tool": map[string]any{
					"type":        "string",
					"enum":        subTools,
					"description": description,
				},
				"instruction": map[string]any{
					"type":        "string",
					"description": "这次联网探索要完成的事，用自然语言描述，例如：查一下今天的 AI 新闻；去 B 站看看有趣的游戏视频；把这个文件 @D.012 发给群「测试群」",
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
	case networkBilibilViewerTool:
		text, err := BilibiliViewer.RunInstruction(instruction)
		return networkExploreResult("哔哩观影", text, err)
	case networkQQSendTool:
		if !QQMessenger.Ready() {
			return ToolResult{Text: "QQ 未就绪：暂时无法向 QQ 发送数据"}
		}
		text, err := QQMessenger.Run(instruction)
		return networkExploreResult("QQ投递", text, err)
	default:
		return ToolResult{Text: "tool 只能取 web_search / bilibil_viewer / qq_send"}
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
