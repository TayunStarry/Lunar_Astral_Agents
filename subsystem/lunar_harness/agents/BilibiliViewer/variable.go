package BilibiliViewer

// variable.go 哔哩观影的常量与全局状态集中管理。
//
// 数量类机制（候选几个、切片几段、记忆多长）都在这里定标；
// 个人偏好放在 local_data/bilibili_life.json，不在代码里硬编码。

import (
	"time"
)

// ==== 候选与采集参数 ====

const (
	// candidateMin / candidateMax 本轮候选视频数量的固定范围（含边界）
	candidateMin = 4
	candidateMax = 8
	// collectCommentLimit 每个候选采集的评论条数上限
	collectCommentLimit = 30
	// collectCommentMaxRunes 单条评论超过该长度直接硬截断
	collectCommentMaxRunes = 200
	// collectDanmakuLimit 每个候选采集的实时弹幕条数上限
	collectDanmakuLimit = 30
	// collectPageCount 候选详情页评论翻页数（凑够 collectCommentLimit 条）
	collectPageCount = 2
	// collectPageSize 单页评论条数
	collectPageSize = 20
)

// ==== 观看切片参数 ====

const (
	// watchSliceMin / watchSliceMax 单个视频切片数的硬边界
	watchSliceMin = 2
	watchSliceMax = 8
	// watchSliceDefault 模型未给出可用切片数时的默认值
	watchSliceDefault = 5
	// sliceRetryLimit 单个切片失败后在下一个 Tick 的重试次数
	sliceRetryLimit = 1
	// mentionMinCount / mentionMaxCount 从特别关注里随机 @ 的人数范围
	mentionMinCount = 1
	mentionMaxCount = 3
	// replyPause 两次被动回应之间的间隔，避免触发 B 站频率限制
	replyPause = 1500 * time.Millisecond
	// embeddingTimeout 候选向量匹配的总超时；超时退回关键词打分
	embeddingTimeout = 25 * time.Second
	// inboxMaxAge 只处理最近 7 天的未读，避免首次启用时把陈年消息全回一遍
	inboxMaxAge = 7 * 24 * time.Hour
	// coverMaxDim 封面送给多模态模型前的长边上限
	coverMaxDim = 480
	// coverJPEGQuality 封面统一转码为 JPEG 的质量
	coverJPEGQuality = 82
	// fallbackCoverMaxBytes 封面下载体积上限（4MB）
	fallbackCoverMaxBytes = 4 << 20
	// modelHTTPRetries 模型返回瞬时错误（HTTP 5xx / 网络失败）时的额外重试次数
	modelHTTPRetries = 2
	// modelRetryBackoff 模型瞬时错误重试的基础退避
	modelRetryBackoff = time.Second
	// lifeLoopMaxRetries 文本类模型调用命中重复输出后的额外重试次数
	lifeLoopMaxRetries = 3
	// reviewAttempts 审核调用的尝试次数（首次带封面，其余纯文本兜底）
	reviewAttempts = 3
	// dynamicMaxRunes 动态正文长度上限
	dynamicMaxRunes = 320
)

// ==== 动态评论区参数 ====

const (
	// dynamicCommentMaxRunes 单条动态评论里评价正文的长度上限
	dynamicCommentMaxRunes = 300
	// dynamicCommentReadyWait 发布动态后等待评论区可用的最长时间（动态可能仍在审核/索引）
	dynamicCommentReadyWait = 120 * time.Second
	// dynamicCommentReadyPoll 等待评论区可用时的轮询间隔
	dynamicCommentReadyPoll = 3 * time.Second
	// dynamicCommentGap 动态评论区逐条评论之间的间隔，避免频率限制与审核叠加
	dynamicCommentGap = 4 * time.Second
	// dynamicCommentAttempts 评论接口报错时的最多尝试次数
	dynamicCommentAttempts = 3
	// dynamicCommentReadbackWait 发出评论后回读评论区的等待时间
	dynamicCommentReadbackWait = 5 * time.Second
)

// ==== 模型名称 ====

const (
	// modelPrimary 主推理模型：审核、评价、动态与被动回应
	modelPrimary = "primary_reasoning"
	// modelLight 轻量模型：视频印象整理
	modelLight = "light_reasoning"
)

// ==== 记忆库格式 ====

const (
	// memoryTagPrefix 哔哩观影记忆在统一记忆库里的固定前缀（记忆条目格式）
	memoryTagPrefix = "[B站视频]"
	// memorySource 写入统一记忆库时的来源标记（用于取回本智能体写入的条目）
	memorySource = "bilibili_viewer"
	// memoryTextMaxRunes 单条人格化记忆正文的长度上限
	memoryTextMaxRunes = 240
)

// mechanicalMemoryMarkers 机械过程文本 / 运行统计的特征词。
//
// 记忆库只存人格化的印象、感受与记忆；命中这些词的正文既不写入，也会在启动时清理。
var mechanicalMemoryMarkers = []string{
	"60 秒切片", "切片的理解", "请合成为", "按时间顺序给出", "【第", "内容概述",
	"画面理解", "帧号", "Frame ", "时间轴", "这一轮看了", "条 B 站互动",
	"发了一条观影动态", "UP主：",
}

// ==== 全局状态 ====

var (
	// manager 进程内唯一的观影管理器（同一时刻只允许一个 Tick 任务）
	manager = &lifeManager{}
)
