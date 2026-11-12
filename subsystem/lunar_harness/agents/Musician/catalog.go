package Musician

// catalog.go 演奏者的能力目录（供控制台「工具列表」屏展示）。
//
// 演奏者不向模型暴露 function calling 工具：它由对话者的 creative_work
//（type=musician）按次调度，内部切换到 music_generation 模型生成旋律。
// 因此这里只保留能力展示条目，不向 toolreg 登记「没有执行器」的假工具。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回演奏者的能力清单。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: agentID, Name: roleLabel,
		Note: "音乐智能体：主模型构思作品描述，music_generation 模型生成旋律并渲染演奏音频。",
		Tools: []agentinfo.Tool{
			{Name: toolMusicGeneration, Description: "加载音乐生成模型产出旋律，完成后卸载并恢复主模型"},
		},
	}}
}
