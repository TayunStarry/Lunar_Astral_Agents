package BilibiliViewer

// tool_catalog.go 哔哩观影的能力目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/engine/agentinfo"

// ToolCatalog 返回哔哩观影的内部能力。
//
// 哔哩观影是「真实浏览器 + 自主生活链」智能体，不使用 function calling；
// 这里列出它在一轮生活中实际会执行的动作。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "bilibili_viewer", Name: "哔哩观影", Source: "agents/BilibiliViewer",
		Note: "真实浏览器自主生活链（非 function calling）",
		Tools: []agentinfo.Tool{
			{Name: "watch_videos", Description: "按偏好与欲求词挑选视频，拖滚动条观看并理解画面与内容。"},
			{Name: "read_comments", Description: "读取视频热门评论，作为写评论与判断氛围的参考。"},
			{Name: "post_comment", Description: "在视频评论区发布一条符合人设的评论。"},
			{Name: "share_dynamic", Description: "发布带 @ 目标的分享评论或动态。"},
			{Name: "reply_inbox", Description: "处理「回复我的 / @我的」未读消息并在原帖回应。"},
			{Name: "write_diary", Description: "把本轮经历写入日记：动态正文、观看记录与长期记忆。"},
		},
	}}
}
