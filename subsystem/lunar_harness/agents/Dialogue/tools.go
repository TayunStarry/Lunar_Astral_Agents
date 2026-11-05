package Dialogue

// tools.go 对话者的工具声明与执行器注册：schema + 说明 + 所属智能体 ID（dialogue）。
//
// 按四层定义，「工具声明」与「工具执行器」都归智能体：本包在 init 中把执行器
// 注册进 protocol/toolreg（实现见 tool_*.go），跨链能力由装配层以函数变量注入，
// 本包不导入任何链。动态枚举（qq_send 就绪后才出现、扩展应用清单）通过
// toolreg.RegisterProvider 在查询时重新计算，避免 init 时把状态固化。

import (
	"context"
	"strings"

	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/engine/resourcehub"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

const (
	// ToolDispatchTechnique 技法创作聚合工具名
	ToolDispatchTechnique = "dispatch_technique"
	// ToolFetchResources 统一资源工具名
	ToolFetchResources = "fetch_resources"
	// ToolLifePlanning 生活规划聚合工具名
	ToolLifePlanning = "life_planning"
	// ToolNetworkExplore 联网探索聚合工具名
	ToolNetworkExplore = "network_explore"

	// networkWebSearch 网页检索子工具
	networkWebSearch = "web_search"
	// networkBilibili 哔哩观影子工具
	networkBilibili = "bilibili_viewer"
	// networkForum 互助论坛子工具
	networkForum = "mutual_forum"
	// networkQQSend 通讯桥接子工具
	networkQQSend = "qq_send"
)

func init() {
	toolreg.Register("dialogue",
		toolreg.Schema{
			Name:        ToolDispatchTechnique,
			Description: "使用自然语言去调度专用子智能体进行技法创作或文件操作",
			Parameters: agentdispatch.Schema(
				"painter=绘画创作、musician=音乐创作、screenshoter=屏幕截图、file_operations=文件操作（读写、整理、压缩包、脚本运行与多步骤任务编排）",
				[]string{"painter", "musician", "screenshoter", "file_operations"},
				"例如: 画一张赛博朋克风格的自画像；创作一段安静的钢琴曲；截取当前屏幕上弹出的对话框；把文档库里的笔记整理成一篇文章；把 notes 目录打包成压缩包",
				"painter 分支可传入图生图基座图句柄（如 @I.003）；file_operations 分支传入任务涉及的资源句柄；其他分支按任务需要传入目标句柄。",
			),
		},
		toolreg.Schema{
			Name:        ToolFetchResources,
			Description: resourcehub.ToolDescription(),
			Parameters:  resourcehub.ToolSchema(),
		},
		toolreg.Schema{
			Name:        ToolLifePlanning,
			Description: "生活规划：计划表增删改查、查看最近做了什么、记录创作意图",
			Parameters: agentdispatch.Schema(
				"plan_manager=计划管理（新增/删除/改写/查询计划表）；recent_activity=查看最近做了什么；record_intent=记录空闲时的创作意图",
				[]string{"plan_manager", "recent_activity", "record_intent"},
				"plan_manager 用自然语言写清要对计划表做什么（如「把明天下午三点的会议改到四点」「今天还有什么计划」）；recent_activity 可留空；record_intent 写清下一轮生活创作想做什么",
				"计划管理可传计划项标识；recent_activity / record_intent 没有目标句柄时传空数组。",
			),
		},
	)
	toolreg.RegisterProvider("dialogue", networkExploreSchemas)
	toolreg.RegisterProvider("dialogue", crystalRemoteSchemas)
	registerExecutors()
}

// registerExecutors 把工具执行器注册进全局注册表（同名后写覆盖先写）。
func registerExecutors() {
	toolreg.RegisterExecutor(ToolDispatchTechnique, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteDispatchTechnique(args), nil
	})
	toolreg.RegisterExecutor(ToolFetchResources, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteFetchResources(args), nil
	})
	toolreg.RegisterExecutor(ToolLifePlanning, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteLifePlanning(args), nil
	})
	toolreg.RegisterExecutor(ToolNetworkExplore, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteNetworkExplore(args), nil
	})
	toolreg.RegisterExecutor(ToolUseTheProgram, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteUseTheProgram(args), nil
	})
}

// networkExploreSchemas 联网探索工具声明：qq_send 仅在通讯桥接就绪时加入枚举。
func networkExploreSchemas() []toolreg.Schema {
	subTools := []string{networkWebSearch, networkBilibili, networkForum}
	descriptionParts := []string{
		"bilibili_viewer=哔哩观影",
		"mutual_forum=互助论坛",
		"web_search=网页检索",
	}
	if DialogueBridging.Ready() {
		subTools = append(subTools, networkQQSend)
		descriptionParts = append(descriptionParts, "qq_send=向指定 QQ 群/好友发送文本、图片或文件（图片/文件使用句柄）")
	}
	return []toolreg.Schema{{
		Name:        ToolNetworkExplore,
		Description: "使用自然语言去调度专用子智能体进行联网探索",
		Parameters: agentdispatch.Schema(
			strings.Join(descriptionParts, "；"),
			subTools,
			"例如: 查一下今天的 AI 新闻；去 B 站看看有趣的猫咪视频；在互助论坛发帖问问 WebView 超时怎么排查；把这个文件 @D.012 发给群「测试群」",
			"qq_send 分支可传入待发送的图片 / 文件句柄（如 @I.007、@D.012）；其他分支按任务需要传入目标句柄。",
		),
	}}
}
