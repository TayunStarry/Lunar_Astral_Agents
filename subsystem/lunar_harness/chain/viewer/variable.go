package viewer

// variable.go 哔哩观影的常量集中管理。
//
// 数量类机制（候选几个、切片几段、记忆多长）都在这里定标；
// 个人偏好放在 local_data/config/bilibili.json，不在代码里硬编码。

import "time"

// ==== 候选与采集参数 ====

const (
	// collectCommentMaxRunes 单条评论超过该长度直接硬截断
	collectCommentMaxRunes = 512
	// collectPageCount 候选详情页评论翻页数（凑够 watchlistCommentLimit 条）
	collectPageCount = 2
	// collectPageSize 单页评论条数
	collectPageSize = 20
)

// ==== 观看切片参数 ====
//
// 一个视频只随机抽一个 1 分钟片段（不足 1 分钟取全片），做一次视频理解。
const (
	// watchSegmentSeconds 单次观看片段的固定时长（秒）。
	watchSegmentSeconds = 60
	// inboxMaxAge 只处理最近 7 天的未读，避免首次启用时把陈年消息全回一遍
	inboxMaxAge = 7 * 24 * time.Hour
	// dynamicMaxRunes 动态正文长度上限
	dynamicMaxRunes = 512
)

// ==== 视频印象与评论参数 ====

const (
	// watchImpressionCommentRows 视频印象里纳入的评论条数。
	watchImpressionCommentRows = 12
	// watchImpressionDanmakuRows 视频印象里纳入的弹幕条数。
	watchImpressionDanmakuRows = 20
	// watchImpressionIntroRunes 视频印象里简介的长度上限。
	watchImpressionIntroRunes = 400
	// watchImpressionVisualRunes 视频印象里画面理解的长度上限。
	watchImpressionVisualRunes = 800
	// watchRemarkMaxRunes 单条发布评论的长度上限（宁可短，也要是完整的一句话）。
	watchRemarkMaxRunes = 80
	// watchRemarkMaxComments 一次观看最多发布的评论条数。
	watchRemarkMaxComments = 2
	// dynamicCommentMaxRunes 被动消息回应（追评/@）的长度上限。
	dynamicCommentMaxRunes = 512
)

// ==== 待看列表补齐的时间预算 ====
//
// 24 个候选串行：每个都要开页、翻评论、取标签、跑审核模型，再下载 1 段。
// 首次无缓存时一轮可能拖很久，与对话回合争抢资源，因此整阶段必须有上限。

const (
	// watchlistBudgetDefault 整阶段默认预算：常见一轮约 8~12 分钟，
	// 给到 30 分钟留 2~4 倍余量，保证首次无缓存的正常抓取能在预算内跑完。
	watchlistBudgetDefault = 30 * time.Minute
	// watchlistCandidateBudgetDefault 单个候选的子预算：避免一次卡死的开页或模型调用吃掉整轮预算。
	watchlistCandidateBudgetDefault = 2 * time.Minute
	// watchlistCollectShareNum / Den 采集阶段最多用掉总预算的 2/3，
	// 把余下 1/3 留给审核、入队与下载，避免「只采集不落库」导致下一轮从零重来。
	watchlistCollectShareNum = 2
	watchlistCollectShareDen = 3
)

// ==== 选片关键词 ====

const (
	// preferenceKeywordLimit 配置里的静态偏好词参与打分的数量上限
	preferenceKeywordLimit = 8
	// watchDynamicKeywordLimit 观影历史动态主题词参与打分的数量上限
	watchDynamicKeywordLimit = 6
	// watchSearchKeywordLimit 搜索关键词池的数量上限（动态词优先）
	watchSearchKeywordLimit = 12
)

// ==== 观影主题词偏好池 ====
//
// 每看完整一个视频，就用它的标签给偏好池加一次权重；
// 选片时按时间衰减后的权重取高频词，形成「看过 → 主题词 → 下次加权」的闭环。
// viewer.db 的 watch_topic 表只负责行级读写，池上限、衰减与淘汰策略全在这里定标。
const (
	// watchTopicPoolMax 主题词池保留的条目上限（写入时截断，读取时按此上限批量取）
	watchTopicPoolMax = 128
	// watchTopicBump 每个视频看完一次，其主题词增加的权重
	watchTopicBump = 1.0
	// watchTopicHalfLifeDays 主题词权重衰减半衰期（天）
	watchTopicHalfLifeDays = 14
	// watchTopicMinWeight 写入时低于该权重的主题词直接淘汰
	watchTopicMinWeight = 0.05
	// watchTopicActiveMinWeight 选片时低于该权重的主题词不参与打分
	watchTopicActiveMinWeight = 0.30
)

// mechanicalMemoryMarkers 机械过程文本 / 运行统计的特征词。
//
// 记忆库只存人格化的印象、感受与记忆；命中这些词的正文不会被当作记忆正文使用。
var mechanicalMemoryMarkers = []string{
	"60 秒切片", "切片的理解", "请合成为", "按时间顺序给出", "【第", "内容概述",
	"画面理解", "帧号", "Frame ", "时间轴", "这一轮看了", "条 B 站互动",
	"发了一条观影动态", "UP主：",
}

// ==== 模型名称 ====

const (
	// modelPrimary 主推理模型：审核、评价、动态与被动回应
	modelPrimary = "primary_reasoning"
	// modelLight 轻量模型：视频印象整理
	modelLight = "light_reasoning"
)

// 观影记忆的存储、上限与迁移规则见 engine/viewerstore（viewer.db 全部由该包负责）。

// ==== 导出与只读诊断 ====
// bilibili 是链级故障归属 ID，不是智能体身份：它只用于 agentdump / agentrepair
// 的条目登记（与 app/tui_hooks.go 的 roleAgentIDs 白名单一致），因此定义在链侧而非智能体族。
const (
	// chainScopeID 链级导出与只读诊断条目的 ID
	chainScopeID = "bilibili"
	// chainScopeLabel 链级导出与只读诊断条目的显示名
	chainScopeLabel = "哔哩观影"
)
