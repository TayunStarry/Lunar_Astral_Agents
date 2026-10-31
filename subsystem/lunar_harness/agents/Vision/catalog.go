package Vision

// catalog.go 视觉理解智能体的工具目录（供控制台「工具列表」屏展示）。
//
// 工具目录即能力边界：只列出视觉理解链路里的观看与定向提问，
// 文件解析、压缩包与归档操作不属于本智能体。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回视觉理解智能体的工具清单。
func ToolCatalog() []agentinfo.Agent {
	tools := []agentinfo.Tool{
		{Name: ToolDescribe, Description: "观看图片 / 动图句柄并返回内容描述；已有描述时零模型调用直接命中。"},
		{Name: ToolAsk, Description: "对图片 / 动图提出定向问题，只本次作答、不覆盖共用描述。"},
		{Name: ToolWatch, Description: "解读本机视频地址：按时间切片抽帧，逐片观看后合成整体感受。"},
	}
	return []agentinfo.Agent{{
		ID: "vision", Name: "视觉理解", Source: "agents/Vision",
		Note:  "视觉理解智能体：完整持有图片 / 动图 / 视频的视觉副本、切片观看与描述回写管线。",
		Tools: tools,
	}}
}
