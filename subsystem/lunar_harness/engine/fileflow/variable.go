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
	// DeltaDirRel 统一创作存储库目录（相对 LocalDir；由 LocalDir + 本常量推导，不改 config 常量）
	DeltaDirRel = "database"
	// DeltaDBFile 统一创作存储库文件名（文件增量与跨轮编排态共用同一个库）
	DeltaDBFile = "creation.db"
	// legacyJournalFile 旧自由创作 JSON 日志（一次性导入来源，导入后保留原文件）
	legacyJournalFile = "database/life/creative_journal.json"
	// legacyAssignedJournalFile 旧对话者指派 JSON 日志（一次性导入来源，导入后保留原文件）
	legacyAssignedJournalFile = "database/life/creative_assigned.json"
	// deltaOpTable 操作元数据表
	deltaOpTable = "flow_ops"
	// deltaTable 内容增量表
	deltaTable = "flow_deltas"
	// deltaRollbackTable 按流程回退记录表
	deltaRollbackTable = "flow_rollbacks"
	// callTable 调用轮次表（一轮调用 = 一次外部任务输入到一次任务输出）
	callTable = "calls"
	// referenceTable 本轮查询到的信息表
	referenceTable = "references"
	// memoTable 自由创作专属备忘录表
	memoTable = "memos"
	// stepFreeTable 自由创作模式的步骤表
	stepFreeTable = "steps_free"
	// stepAssignTable 对话者指定模式的步骤表
	stepAssignTable = "steps_assign"
	// errorFreeTable 自由创作模式的错误表
	errorFreeTable = "errors_free"
	// errorAssignTable 对话者指定模式的错误表
	errorAssignTable = "errors_assign"
	// changeTable 内容级变更的可见窗口表（最新 10 次）
	changeTable = "change"
	// changeBaselineTable 更久远变更合并后的原始基准表（按文件一行）
	changeBaselineTable = "change_baselines"
	// metaTable 库级元数据表（旧 JSON 日志导入标记）
	metaTable = "store_meta"
	// changeColumns change 表读取列（与 scanChanges 顺序一致）
	changeColumns = `id, op_id, call_id, flow_id, seq, kind, path_rel, path_abs, source_rel,
		locator, line_from, line_to, old_text, new_text, old_hash, new_hash,
		old_size, new_size, binary, binary_note, truncated, created_at`
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
	// maxChangeWindow 单个文件在 change 表中保留的可见变更次数（更久远的合并进原始基准）
	maxChangeWindow = 10
	// maxRoundCalls 每种模式默认保留的调用轮数（更久远的轮次记录清理，change 不受影响）
	maxRoundCalls = 20
	// metaLegacyJournalCreatorKey 自由创作 JSON 日志已导入标记键
	metaLegacyJournalCreatorKey = "legacy_journal_creator_migrated"
	// metaLegacyJournalAssignedKey 指派 JSON 日志已导入标记键
	metaLegacyJournalAssignedKey = "legacy_journal_assigned_migrated"
	// manifestFile 快照清单文件名
	manifestFile = "manifest.json"
	// rollbackFile 回滚结果文件名
	rollbackFile = "rollback.json"
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
		`CREATE TABLE IF NOT EXISTS ` + metaTable + ` (
			key    TEXT PRIMARY KEY,
			value  TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS ` + callTable + ` (
			call_id     TEXT PRIMARY KEY,
			mode        TEXT    NOT NULL DEFAULT '',
			status      TEXT    NOT NULL DEFAULT 'running',
			summary     TEXT    NOT NULL DEFAULT '',
			started_at  INTEGER NOT NULL,
			ended_at    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_calls_mode ON ` + callTable + `(mode, started_at)`,
		`CREATE TABLE IF NOT EXISTS "` + referenceTable + `" (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			content     TEXT    NOT NULL,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_references_call_content ON "` + referenceTable + `"(call_id, content)`,
		`CREATE TABLE IF NOT EXISTS ` + memoTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			content     TEXT    NOT NULL,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_memos_call_content ON ` + memoTable + `(call_id, content)`,
		`CREATE TABLE IF NOT EXISTS ` + stepFreeTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			agent       TEXT    NOT NULL DEFAULT '',
			task        TEXT    NOT NULL DEFAULT '',
			expects     TEXT    NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_steps_free_call_ord ON ` + stepFreeTable + `(call_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS ` + stepAssignTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			agent       TEXT    NOT NULL DEFAULT '',
			task        TEXT    NOT NULL DEFAULT '',
			expects     TEXT    NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_steps_assign_call_ord ON ` + stepAssignTable + `(call_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS ` + errorFreeTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			message     TEXT    NOT NULL,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_errors_free_call_ord ON ` + errorFreeTable + `(call_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS ` + errorAssignTable + ` (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			call_id     TEXT    NOT NULL REFERENCES ` + callTable + `(call_id) ON DELETE CASCADE,
			ordinal     INTEGER NOT NULL DEFAULT 0,
			message     TEXT    NOT NULL,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_errors_assign_call_ord ON ` + errorAssignTable + `(call_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS ` + changeTable + ` (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			op_id         INTEGER NOT NULL REFERENCES ` + deltaOpTable + `(id) ON DELETE CASCADE,
			call_id       TEXT    NOT NULL DEFAULT '',
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
		`CREATE INDEX IF NOT EXISTS idx_change_path ON ` + changeTable + `(path_rel, op_id)`,
		`CREATE INDEX IF NOT EXISTS idx_change_call ON ` + changeTable + `(call_id)`,
		`CREATE TABLE IF NOT EXISTS ` + changeBaselineTable + ` (
			path_rel           TEXT PRIMARY KEY,
			merged_count       INTEGER NOT NULL DEFAULT 0,
			first_op_id        INTEGER NOT NULL DEFAULT 0,
			last_op_id         INTEGER NOT NULL DEFAULT 0,
			first_flow_id      TEXT    NOT NULL DEFAULT '',
			base_before_hash   TEXT    NOT NULL DEFAULT '',
			base_before_size   INTEGER NOT NULL DEFAULT 0,
			latest_after_hash  TEXT    NOT NULL DEFAULT '',
			latest_after_size  INTEGER NOT NULL DEFAULT 0,
			first_at           INTEGER NOT NULL DEFAULT 0,
			last_at            INTEGER NOT NULL DEFAULT 0
		)`,
	}

	// deltaStoreMu 保护增量库连接（LocalDir 变化时按新路径重开）
	deltaStoreMu sync.Mutex
	// deltaDB 增量库连接
	deltaDB *sql.DB
	// deltaDBPath 当前连接对应的库路径
	deltaDBPath string
)
