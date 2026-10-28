package BilibiliViewer

// tool_catalog.go 哔哩观影的能力目录（供控制台「工具列表」屏展示）。

import "LunarSubsystem/LunarHarness/protocol/agentinfo"

// ToolCatalog 返回哔哩观影的内部能力。
//
// 哔哩观影是「真实浏览器 + Tick 状态机」智能体，不使用 function calling；
// 这里列出它实际会执行的动作：启动阶段一次性完成，观看阶段每个 Tick 一步。
func ToolCatalog() []agentinfo.Agent {
	return []agentinfo.Agent{{
		ID: "bilibili_viewer", Name: "哔哩观影", Source: "agents/BilibiliViewer",
		Note: "真实浏览器 + Tick 状态机（非 function calling）",
		Tools: []agentinfo.Tool{
			{Name: "reply_passive", Description: "被动回应收到的 @ 与追评（页面操作，不主动评论他人视频）。"},
			{Name: "build_keywords", Description: "结合对话指令、长期记忆与配置关键词生成本轮搜索词。"},
			{Name: "preselect_videos", Description: "预选 4-8 个候选视频。"},
			{Name: "collect_video_info", Description: "采集标题/标签/简介/时长/发布时间/UP主/30 条评论/30 条弹幕。"},
			{Name: "build_impressions", Description: "用 light_reasoning 生成结构化视频印象。"},
			{Name: "rank_videos", Description: "嵌入相似度 + 热度加权，综合匹配度排序。"},
			{Name: "review_and_evaluate", Description: "用 primary_reasoning 审核合规性、生成视频评价与切片数。"},
			{Name: "publish_dynamic", Description: "先发布观影动态，并在动态评论区逐条输出视频评价与 URL。"},
			{Name: "share_with_mentions", Description: "额外一条 @分享评论，随机 @ 特别关注 1-3 人。"},
			{Name: "watch_slice_tick", Description: "之后每个 Tick 观看并理解一个 60 秒切片，输出片段理解。"},
			{Name: "merge_and_store", Description: "合并单个视频全部切片理解并写入记忆库。"},
			{Name: "memory_crud", Description: "在统一记忆库中逐条枚举、查看、新增、修改、删除 [B站视频] 条目。"},
		},
	}}
}
