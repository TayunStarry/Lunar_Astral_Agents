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
	// ToolFetchResources 统一资源工具名
	ToolFetchResources = "fetch_resources"
	// ToolNetworkExplore 联网探索聚合工具名
	ToolNetworkExplore = "network_explore"

	// networkWebSearch 网页检索子工具
	networkWebSearch = "web_search"
	// networkQQSend 通讯桥接子工具
	networkQQSend = "qq_send"
)

func init() {
	toolreg.Register("dialogue",
		toolreg.Schema{
			Name:        ToolFetchResources,
			Description: resourcehub.ToolDescription(),
			Parameters:  resourcehub.ToolSchema(),
		},
		toolreg.Schema{
			Name:        ToolLifePlanning,
			Description: "生活规划：愿望管理、观影与互助论坛",
			Parameters: agentdispatch.Schema(
				"write_wish=用自然语言描述你的愿望或什么时候想去做什么事；"+
					"wish_complete=用自然语言描述你的什么愿望已达成；"+
					"bilibili_viewer=哔哩观影；mutual_forum=互助论坛",
				lifeTypeOrder,
				"bilibili_viewer / mutual_forum 用自然语言说明想看或想聊什么；愿望类指令不使用资源句柄。",
				"愿望类 handle 传空数组；观影与论坛没有目标时传空数组。",
			),
		},
		toolreg.Schema{
			Name:        ToolCreativeWork,
			Description: "创作任务：故事、文件编辑、演绎、绘画、音乐或截图，用 type 选择分支",
			Parameters: agentdispatch.Schema(
				"story=故事创作（新建故事或续写下一章）；"+
					"files=文件编辑（读写、整理、压缩包、脚本运行、内容回滚）；"+
					"storyboard=演绎（把故事拆成分镜并逐镜绘制）；"+
					"painter=绘画创作；musician=音乐创作；screenshoter=屏幕截图",
				creativeTypeOrder,
				"例如：story 写一个新故事，题材是……；files 把 documents/说明.md 的错别字改掉；"+
					"storyboard 把《雨港灯塔》第一章演绎成分镜；painter 画一张赛博朋克自画像；"+
					"musician 创作一段安静的钢琴曲；screenshoter 截取当前屏幕",
				"story 传故事正文或参考资料句柄；files 传任务涉及的文件句柄；"+
					"storyboard 传故事正文文件句柄；painter 可传参考图句柄（如 @I.003）；"+
					"musician / screenshoter 没有目标时传空数组。",
			),
		},
	)
	toolreg.RegisterProvider("dialogue", networkExploreSchemas)
	toolreg.RegisterProvider("dialogue", crystalRemoteSchemas)
	registerExecutors()
}

// registerExecutors 把工具执行器注册进全局注册表（同名后写覆盖先写）。
func registerExecutors() {
	toolreg.RegisterExecutor(ToolFetchResources, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteFetchResources(args), nil
	})
	toolreg.RegisterExecutor(ToolLifePlanning, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteLifePlanning(args), nil
	})
	toolreg.RegisterExecutor(ToolCreativeWork, func(_ context.Context, args map[string]any) (any, error) {
		return ExecuteCreativeWork(args), nil
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
	subTools := []string{networkWebSearch}
	descriptionParts := []string{"web_search=网页检索"}
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
			"例如: 查一下今天的 AI 新闻；把这个文件 @D.012 发给群「测试群」",
			"qq_send 分支可传入待发送的图片 / 文件句柄（如 @I.007、@D.012）；web_search 按任务需要传入目标句柄。",
		),
	}}
}
