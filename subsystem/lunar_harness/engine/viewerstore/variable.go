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

// ==== 历史 JSON 迁移 ====

const (
	// inboxSeenJSONFile 旧版消息去重键文件（位于 DataDir）
	inboxSeenJSONFile = "inbox.json"
	// seenVideoJSONFile 旧版已看记录文件（位于 DataDir）
	seenVideoJSONFile = "seen.json"
	// inboxSeenImportMark / seenVideoImportMark viewer_meta 中的一次性迁移标记
	inboxSeenImportMark = "migrate:inbox_seen:v1"
	seenVideoImportMark = "migrate:seen_video:v1"
	// legacyImportAbsent 源文件不存在时的标记值（避免每次打开都去 stat）
	legacyImportAbsent = "absent"
)
