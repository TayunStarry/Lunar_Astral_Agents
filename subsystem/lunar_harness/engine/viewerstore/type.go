package viewerstore

// type.go 观影记忆存储层的共享类型定义。
//
// 存储层只负责 viewer.db 的读写与上限维持；提示词渲染、选片口味重算、启动时序
// 等编排逻辑仍留在 chain/viewer。

// Observation 一条近期观感（viewer_observation 表的一行）。
//
// 拆分字段存储，渲染提示词时再由编排层拼成一行正文。
type Observation struct {
	BV         string `json:"bv"`
	Title      string `json:"title,omitempty"`
	URL        string `json:"url,omitempty"`
	Topics     string `json:"topics,omitempty"`
	Tone       string `json:"tone,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	UpdatedAt  int64  `json:"updated_at,omitempty"`
}

// Round 一轮观影的追溯记录（viewer_round 表的一行）。
type Round struct {
	ID         string `json:"id"`
	BVIDs      string `json:"bvids,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	CreatedAt  int64  `json:"created_at,omitempty"`
}

// LegacyMemoryEntry 旧记忆库里待迁移的一条历史观影记忆。
type LegacyMemoryEntry struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Source    string `json:"source,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

// LegacyMemorySource 迁移所需的旧记忆库端口：只含分页扫描与删除两项能力。
//
// 由编排层（chain/viewer）注入，存储层不直接依赖 engine/memory。
type LegacyMemorySource interface {
	// ListText 按页读取文本记忆，返回当页条目与总条数。
	//
	// 记忆库不可用（未初始化、加载失败）时必须返回 error，
	// 不能以「空页」冒充「没有历史条目」，否则迁移会被误判为已完成。
	ListText(offset, limit int) ([]LegacyMemoryEntry, int, error)
	// DeleteEntry 删除一条已完成迁移的源条目。
	DeleteEntry(id string) error
}

// WatchStatus 待看视频的审核状态。
type WatchStatus string

// SegmentInput 入队 / 重切时提交的一个片段（序号由存储层按时间顺序分配）。
type SegmentInput struct {
	StartSec   int    `json:"start_sec"`
	EndSec     int    `json:"end_sec"`
	Path       string `json:"path,omitempty"`
	Downloaded bool   `json:"downloaded,omitempty"`
}

// WatchCandidate 入队时提交的一条「视频印象」与片段清单。
//
// Segments 可以为空：审核通过后先入库，片段在下载阶段再用 ReplaceSegments 补写。
type WatchCandidate struct {
	BV          string         `json:"bv"`
	Title       string         `json:"title,omitempty"`
	Intro       string         `json:"intro,omitempty"`
	AuthorUID   string         `json:"author_uid,omitempty"`
	AuthorName  string         `json:"author_name,omitempty"`
	Comments    []string       `json:"comments,omitempty"`
	DurationSec int            `json:"duration_sec,omitempty"`
	Segments    []SegmentInput `json:"segments,omitempty"`
}

// WatchVideo 待看视频列表的一行（watch_video）。
//
// 时间字段一律为 Unix 秒；QueueSeq 是入队顺序（单调整数，删除归档也不回收）。
type WatchVideo struct {
	BV           string      `json:"bv"`
	Title        string      `json:"title,omitempty"`
	Intro        string      `json:"intro,omitempty"`
	AuthorUID    string      `json:"author_uid,omitempty"`
	AuthorName   string      `json:"author_name,omitempty"`
	Comments     []string    `json:"comments,omitempty"`
	DurationSec  int         `json:"duration_sec,omitempty"`
	Status       WatchStatus `json:"status"`
	RejectReason string      `json:"reject_reason,omitempty"`
	QueueSeq     int64       `json:"queue_seq,omitempty"`
	QueuedAt     int64       `json:"queued_at,omitempty"`
	ReviewedAt   int64       `json:"reviewed_at,omitempty"`
	Completed    bool        `json:"completed,omitempty"`
	CompletedAt  int64       `json:"completed_at,omitempty"`
	Archived     bool        `json:"archived,omitempty"`
	ArchivedAt   int64       `json:"archived_at,omitempty"`
}

// WatchSegment 待看视频片段的一行（watch_segment）。
type WatchSegment struct {
	ID         int64  `json:"id"`
	BV         string `json:"bv"`
	Index      int    `json:"index"`
	StartSec   int    `json:"start_sec"`
	EndSec     int    `json:"end_sec"`
	Path       string `json:"path,omitempty"`
	Downloaded bool   `json:"downloaded,omitempty"`
	Watched    bool   `json:"watched,omitempty"`
	WatchedAt  int64  `json:"watched_at,omitempty"`
}

// EnqueueReport 批量入队的对账信息；同 BV 已存在计入 Skipped。
type EnqueueReport struct {
	Inserted    int      `json:"inserted"`
	Skipped     int      `json:"skipped"`
	InsertedBVs []string `json:"inserted_bvs,omitempty"`
}

// SegmentWatchResult 标记片段已看后的视频级状态。
type SegmentWatchResult struct {
	// VideoCompleted 该视频的全部片段是否都已看完
	VideoCompleted bool `json:"video_completed"`
	// CompletedVideos 当前完整视频累计数（含已归档）
	CompletedVideos int `json:"completed_videos"`
}

// MigrationReport 历史记忆迁移对账。
type MigrationReport struct {
	Source     int    `json:"source"`
	Imported   int    `json:"imported"`
	Discarded  int    `json:"discarded"`
	Failed     int    `json:"failed"`
	BackupPath string `json:"backup_path,omitempty"`
}
