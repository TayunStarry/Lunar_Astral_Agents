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

// MigrationReport 历史记忆迁移对账。
type MigrationReport struct {
	Source     int    `json:"source"`
	Imported   int    `json:"imported"`
	Discarded  int    `json:"discarded"`
	Failed     int    `json:"failed"`
	BackupPath string `json:"backup_path,omitempty"`
}
