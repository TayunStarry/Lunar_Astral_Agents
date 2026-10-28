package DialogueBridging

// tool_catalog.go 通讯桥接的内部能力目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回通讯桥接的执行能力。
//
// 该智能体对对话者只暴露自然语言入口，内部由规划模型解析目标后再调用
// 下列私有能力，因此这里展示的是「智能体实际会执行的工具」，不是 function calling Schema。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "dialogue_bridging", Name: "通讯桥接", Source: "agents/DialogueBridging",
		Note: "规划式执行：一次需求可指定多个目标，按顺序逐个投递",
		Tools: []agentinfo.Tool{
			{Name: "resolve_target", Description: "把群号 / 群名 / QQ 号 / 昵称 / 备注解析为可投递目标，名称歧义时报错。"},
			{Name: "send_text", Description: "向一个目标发送文本消息。"},
			{Name: "send_images", Description: "向一个目标发送图片句柄对应的图片（单条最多 9 张，超出自动分批）。"},
			{Name: "send_files", Description: "向一个目标发送文件句柄对应的文档 / 视频 / 音频 / 表格。"},
			{Name: "send_batch", Description: "解析出多个投递目标后，逐个目标依次执行文本 / 图片 / 文件投递，并汇总每个目标的成败。"},
		},
	}}
}
