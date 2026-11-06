package viewerstore

// variable.go 观影记忆存储层的常量、上限与迁移识别规则集中管理。

import "regexp"

// ==== 数据库 ====

const (
	// dbName 观影记忆库文件名（位于 local_data/database/bilibili）
	dbName = "viewer.db"
	// maxOpenConns / maxIdleConns 单连接策略（避免 sqlite 并发写锁冲突）
	maxOpenConns = 1
	maxIdleConns = 1
)

// ==== 单行槽 / 内部状态 ====

const (
	// profileSlotKey 选片口味单行表的固定 key
	profileSlotKey = "selection"
	// stanceSlotKey 评论倾向单行表的固定 key
	stanceSlotKey = "stance"
	// migrationMetaKey 历史记忆迁移完成标记
	migrationMetaKey = "legacy_memory_migrated_v1"
	// migrationDoneValue 迁移完成标记的取值
	migrationDoneValue = "1"
)

// ==== 记忆上限与长度 ====

const (
	// ObservationMax 近期观感条数上限（超出按最旧归档）
	ObservationMax = 6
	// RoundMax 观影轮次追溯表条数上限（超出按最旧归档）
	RoundMax = 20
	// ProfileTopicMax 选片口味里保留的主题词数量
	ProfileTopicMax = 6
	// TextMaxRunes 单条人格化记忆正文的长度上限（硬截断统一从这里起步）
	TextMaxRunes = 512
	// ConclusionMaxRunes 记忆库里「结论」部分的长度上限
	ConclusionMaxRunes = TextMaxRunes
	// TopicMax 单条观影观感里保留的主题词数量
	TopicMax = 4
)

// ==== 展示标记（旧的记忆正文也用它）====

const (
	// SelectionMarker 选片口味正文的展示标记
	SelectionMarker = "选片口味："
	// StanceMarker 评论倾向正文的展示标记
	StanceMarker = "评论倾向："
)

// ==== 待看视频列表 ====

const (
	// WatchStatusPending 已入队，等待审核
	WatchStatusPending WatchStatus = "pending"
	// WatchStatusApproved 审核通过，可以进入播放队列
	WatchStatusApproved WatchStatus = "approved"
	// WatchStatusRejected 审核未通过（原因写入 reject_reason）
	WatchStatusRejected WatchStatus = "rejected"
	// WatchSegmentMax 单个视频的片段数量上限（下限为 0，表示还没切）
	WatchSegmentMax = 5
	// WatchCommentsMax 单条视频保留的评论摘录数量上限
	WatchCommentsMax = 20
	// WatchIntroMaxRunes 视频简介入库长度上限
	WatchIntroMaxRunes = 600
	// WatchRejectReasonMaxRunes 审核未通过原因的长度上限
	WatchRejectReasonMaxRunes = 200
)

// watchVideoColumns 待看视频表的固定列序（查询与 Scan 共用，避免两处漂移）。
const watchVideoColumns = `bv, title, intro, author_uid, author_name, comments,
		duration_sec, status, reject_reason, queue_seq, queued_at, reviewed_at,
		completed, completed_at, archived, archived_at`

// ==== 归档原因 ====

const (
	// archiveObservationOverflow 观感超额的归档原因
	archiveObservationOverflow = "observation_overflow"
	// archiveRoundOverflow 轮次超额的归档原因
	archiveRoundOverflow = "round_overflow"
)

// ==== 历史迁移识别 ====

const (
	// memorySource 统一记忆库里旧观影条目的来源标记（迁移识别用）
	memorySource = "bilibili_viewer"
)

var (
	// memoryKinds 允许写入的记忆类别（用于区分新格式的 [类别] 与旧格式的 [前缀]）
	memoryKinds = map[string]bool{
		"印象": true, "感受": true, "评价": true, "习惯": true,
		"问题": true, "吐槽": true, "人物": true, "线索": true,
	}
	// legacyViewerPrefixes 旧版本写入观影记忆时拼接的展示前缀。
	//
	// 只用于识别历史数据：不做「首段不是类别就算旧观影」这类否定式判断，
	// 否则会把对话记忆等其它链路的正文也误认成观影条目。
	legacyViewerPrefixes = []string{"B站视频"}
	// dialoguePrefixLabels 对话链路写入正文时的首段标签（[用户: …] / [群聊: …]）。
	//
	// 这些条目一律不进入观影记忆的迁移候选池。
	dialoguePrefixLabels = []string{"用户", "群聊"}
	// legacyTitleRe / legacyURLRe 从旧正文里提取标题与链接
	legacyTitleRe = regexp.MustCompile(`《([^》]+)》`)
	legacyURLRe   = regexp.MustCompile(`https?://[^\s｜]+`)
)

// mechanicalMemoryMarkers 机械过程文本 / 运行统计的特征词。
//
// 记忆库只存人格化的印象、感受与记忆；命中这些词的正文既不写入，也会在迁移时丢弃。
var mechanicalMemoryMarkers = []string{
	"60 秒切片", "切片的理解", "请合成为", "按时间顺序给出", "【第", "内容概述",
	"画面理解", "帧号", "Frame ", "时间轴", "这一轮看了", "条 B 站互动",
	"发了一条观影动态", "UP主：",
}
