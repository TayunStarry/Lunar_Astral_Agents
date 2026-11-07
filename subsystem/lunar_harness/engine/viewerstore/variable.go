package viewerstore

// variable.go 观影记忆存储层的常量与上限集中管理。

// ==== 数据库 ====

const (
	// dbName 观影记忆库文件名（位于 local_data/database/bilibili）
	dbName = "viewer.db"
	// maxOpenConns / maxIdleConns 单连接策略（避免 sqlite 并发写锁冲突）
	maxOpenConns = 1
	maxIdleConns = 1
)

// ==== 长度上限 ====

const (
	// TextMaxRunes 单条人格化记忆正文的长度上限（硬截断统一从这里起步）
	TextMaxRunes = 512
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
	// WatchTopicsMax 单个视频保留的主题词数量上限
	WatchTopicsMax = 8
	// WatchTopicMaxRunes 单个主题词的长度上限
	WatchTopicMaxRunes = 32
)

// watchVideoColumns 待看视频表的固定列序（查询与 Scan 共用，避免两处漂移）。
const watchVideoColumns = `bv, title, intro, author_uid, author_name, comments,
		duration_sec, status, reject_reason, queue_seq, queued_at, reviewed_at,
		completed, completed_at, archived, archived_at, topics`

// watchVideoColumnAdds 旧库启动时自动补齐的列。
//
// 只在 PRAGMA table_info 里缺少该列时执行一次；ADD COLUMN 对既有行使用默认值，
// 不重写数据，因此对已存在的历史行完全向后兼容。
var watchVideoColumnAdds = []columnAdd{
	{Column: "topics", SQL: `ALTER TABLE watch_video ADD COLUMN topics TEXT NOT NULL DEFAULT ''`},
}

// ==== 观看主题词池（个性化反馈） ====
//
// 每看完整一个视频，就用它印象里的主题词给偏好池加一次权重；
// 选片时按时间衰减后的权重取高频词，形成「看过 → 主题词 → 下次加权」的闭环。
const (
	// watchTopicPoolMax 主题词池保留的条目上限（写入时截断）
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
