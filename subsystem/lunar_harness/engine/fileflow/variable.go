package fileflow

// variable.go 文件操作流程的常量与全局变量（业务逻辑不得写在本文件）。

import (
	"database/sql"
	"sync"
	"sync/atomic"
)

const (
	// SnapshotRootRel 快照与流程追踪的统一根目录（相对 LocalDir）
	SnapshotRootRel = "database/file_ops"
	// DeltaDirRel 内容级增量记录库目录（相对 LocalDir；由 LocalDir + 本常量推导，不改 config 常量）
	DeltaDirRel = "database/fileflow"
	// DeltaDBFile 内容级增量记录库文件名
	DeltaDBFile = "flow_ops.db"
	// deltaOpTable 操作元数据表
	deltaOpTable = "flow_ops"
	// deltaTable 内容增量表
	deltaTable = "flow_deltas"
	// deltaRollbackTable 按流程回退记录表
	deltaRollbackTable = "flow_rollbacks"
	// maxDeltaFragmentBytes 单条文本片段的字节上限（超出按块截断并标记 truncated）
	maxDeltaFragmentBytes = 4 << 10
	// maxDeltaReadBytes 单个文件参与文本差异计算的读取上限（超过直接按二进制记录哈希）
	maxDeltaReadBytes = 8 << 20
	// maxDeltaHunks 单次修改最多保留的文本差异块数
	maxDeltaHunks = 32
	// maxDeltaTreeItems 目录批量写最多列举的变更文件数
	maxDeltaTreeItems = 40
	// maxDeltaFlows 增量表默认保留的流程数
	maxDeltaFlows = 100
	// maxDeltaAgeDays 增量表默认保留天数
	maxDeltaAgeDays = 30
	// manifestFile 快照清单文件名
	manifestFile = "manifest.json"
	// rollbackFile 回滚结果文件名
	rollbackFile = "rollback.json"
	// flowTraceFile 完整流程追踪文件名
	flowTraceFile = "flow.json"
	// blobsDir 快照副本子目录
	blobsDir = "blobs"
	// maxSnapshotFlows 保留的流程快照目录数量（超出时删除最旧的）
	maxSnapshotFlows = 20
	// maxSnapshotBytes 单个文件允许抓取快照的上限（超过则取消写入并显式上报）
	maxSnapshotBytes = 64 << 20
	// maxSnapshotTreeFiles 目录快照允许包含的文件数上限
	maxSnapshotTreeFiles = 2000
	// maxSnapshotTreeBytes 目录快照允许包含的总字节上限
	maxSnapshotTreeBytes = 256 << 20
)

var (
	// flowSeq 流程 ID 序号（同一秒内多次运行也不会重名）
	flowSeq atomic.Int64

	// deltaSchema 增量记录库建表语句（操作元数据 / 内容增量 / 回退记录）
	deltaSchema = []string{
		`CREATE TABLE IF NOT EXISTS ` + deltaOpTable + ` (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			flow_id       TEXT    NOT NULL,
			seq           INTEGER NOT NULL,
			step          INTEGER NOT NULL DEFAULT 0,
			mode          TEXT    NOT NULL DEFAULT '',
			op            TEXT    NOT NULL,
			delta_kind    TEXT    NOT NULL DEFAULT '',
			target        TEXT    NOT NULL DEFAULT '',
			path_rel      TEXT    NOT NULL DEFAULT '',
			path_abs      TEXT    NOT NULL DEFAULT '',
			agent         TEXT    NOT NULL DEFAULT '',
			tool          TEXT    NOT NULL DEFAULT '',
			snapshot      TEXT    NOT NULL DEFAULT '',
			before_sha256 TEXT    NOT NULL DEFAULT '',
			after_sha256  TEXT    NOT NULL DEFAULT '',
			before_size   INTEGER NOT NULL DEFAULT 0,
			after_size    INTEGER NOT NULL DEFAULT 0,
			review_status TEXT    NOT NULL DEFAULT '',
			review_detail TEXT    NOT NULL DEFAULT '',
			rollback_ok   INTEGER NOT NULL DEFAULT 0,
			rollback_error TEXT   NOT NULL DEFAULT '',
			status        TEXT    NOT NULL DEFAULT 'applied',
			created_at    INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_flow_ops_flow_seq ON ` + deltaOpTable + `(flow_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_flow_ops_created ON ` + deltaOpTable + `(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_flow_ops_flow_step ON ` + deltaOpTable + `(flow_id, step)`,
		`CREATE TABLE IF NOT EXISTS ` + deltaTable + ` (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			op_id         INTEGER NOT NULL REFERENCES ` + deltaOpTable + `(id) ON DELETE CASCADE,
			flow_id       TEXT    NOT NULL,
			seq           INTEGER NOT NULL,
			kind          TEXT    NOT NULL,
			path_rel      TEXT    NOT NULL DEFAULT '',
			path_abs      TEXT    NOT NULL DEFAULT '',
			source_rel    TEXT    NOT NULL DEFAULT '',
			locator       TEXT    NOT NULL DEFAULT '',
			line_from     INTEGER NOT NULL DEFAULT 0,
			line_to       INTEGER NOT NULL DEFAULT 0,
			old_text      TEXT    NOT NULL DEFAULT '',
			new_text      TEXT    NOT NULL DEFAULT '',
			old_hash      TEXT    NOT NULL DEFAULT '',
			new_hash      TEXT    NOT NULL DEFAULT '',
			old_size      INTEGER NOT NULL DEFAULT 0,
			new_size      INTEGER NOT NULL DEFAULT 0,
			binary        INTEGER NOT NULL DEFAULT 0,
			binary_note   TEXT    NOT NULL DEFAULT '',
			truncated     INTEGER NOT NULL DEFAULT 0,
			created_at    INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_flow_deltas_flow_seq ON ` + deltaTable + `(flow_id, seq)`,
		`CREATE TABLE IF NOT EXISTS ` + deltaRollbackTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			flow_id     TEXT    NOT NULL,
			target_seq  INTEGER NOT NULL,
			from_seq    INTEGER NOT NULL,
			ok          INTEGER NOT NULL DEFAULT 0,
			detail      TEXT    NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL
		)`,
	}

	// deltaStoreMu 保护增量库连接（LocalDir 变化时按新路径重开）
	deltaStoreMu sync.Mutex
	// deltaDB 增量库连接
	deltaDB *sql.DB
	// deltaDBPath 当前连接对应的库路径
	deltaDBPath string
)
