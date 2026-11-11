package viewer

// variable.go 哔哩观影的常量集中管理。
//
// 数量类机制（候选几个、切片几段、记忆多长）都在这里定标；
// 个人偏好放在 local_data/config/bilibili.json，不在代码里硬编码。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// ==== 候选与采集参数 ====

const (
	// collectCommentLimit 每个候选采集的评论条数上限
	collectCommentLimit = 30
	// collectCommentMaxRunes 单条评论超过该长度直接硬截断
	collectCommentMaxRunes = 512
	// collectDanmakuLimit 每个候选采集的实时弹幕条数上限
	collectDanmakuLimit = 30
	// collectPageCount 候选详情页评论翻页数（凑够 collectCommentLimit 条）
	collectPageCount = 2
	// collectPageSize 单页评论条数
	collectPageSize = 20
)

// ==== 观看切片参数 ====

const (
	// watchSliceMin / watchSliceMax 单个视频切片数的硬边界。
	// 需求只要求 1~5 段，上限直接复用存储层的 WatchSegmentMax，避免审核与下载两套口径。
	watchSliceMin = 1
	watchSliceMax = viewerstore.WatchSegmentMax
	// watchSliceDefault 模型未给出可用切片数时的默认值
	watchSliceDefault = 5
	// inboxMaxAge 只处理最近 7 天的未读，避免首次启用时把陈年消息全回一遍
	inboxMaxAge = 7 * 24 * time.Hour
	// dynamicMaxRunes 动态正文长度上限
	dynamicMaxRunes = 512
)

// ==== 评论触发参数 ====

const (
	// segmentRemarkChance 普通片段的评论触发概率。
	//
	// 每片都发太密集；取 1/4 后，一个 4 段视频平均只有 1 条过程评论，
	// 加上每支视频必发的收束评论，密度接近「真人边看边偶尔说两句」。
	segmentRemarkChance = 0.25
)

// ==== 动态评论区参数 ====

const (
	// dynamicCommentMaxRunes 单条动态评论里评价正文的长度上限
	dynamicCommentMaxRunes = 512
)

// ==== 待看列表补齐的时间预算 ====
//
// 24 个候选串行：每个都要开页、翻评论、跑印象与审核模型，再下载 1~5 段。
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
// 每看完整一个视频，就用它印象里的主题词给偏好池加一次权重；
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
