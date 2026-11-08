package library

// variable.go 资源库的常量与全局状态。

import (
	"database/sql"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/resource"
)

const (
	// Module 日志模块名
	Module = "资源库"
	// maxEditHistory 每个文件在接口中保留的最近操作条数（前端展示窗口）
	maxEditHistory = 10
	// maxRecordsPerFile 每个文件在库中的历史保留条数（超出按序号裁剪最旧记录）
	maxRecordsPerFile = 100
	// maxTotalRecords 全库历史记录总上限（防止文件数量增长导致库无限膨胀）
	maxTotalRecords = 50000
	// maxRecordAgeDays 历史记录最长保留天数
	maxRecordAgeDays = 180
	// cleanupEverySeq 每写入多少次执行一次全库保留策略清理
	cleanupEverySeq = 200
	// coalesceWindowSec 无正文变化的同类操作在该窗口内合并为一条（避免重复读取刷屏）
	coalesceWindowSec = 60
	// maxSnippetRunes 单条新增 / 删除片段的最大字符数
	maxSnippetRunes = 6000
	// maxTreeEntries 单次递归清单的条目上限（防止超大库把响应撑爆）
	maxTreeEntries = 20000
	// maxPageSize 单页状态 / 历史记录条数上限
	maxPageSize = 1000
	// defaultHistoryPageSize 历史记录默认分页大小
	defaultHistoryPageSize = 50
	// defaultDBPageSize 数据库行数据默认每页条数
	defaultDBPageSize = 100
	// maxDBPageSize 数据库行数据单页上限
	maxDBPageSize = 500
	// maxDBCellBytes 二进制单元格转为十六进制时的最大展示字节数
	maxDBCellBytes = 128
	// maxDBQueryRows 只读 SQL 单次返回的最大行数
	maxDBQueryRows = 500
	// dbQueryTimeout 只读 SQL 的最长执行时间
	dbQueryTimeout = 30 * time.Second
	// maxDBSchemaPromptTables 自然语言转 SQL 时随提示词携带的最大表数
	maxDBSchemaPromptTables = 80
	// defaultArchivePageSize 压缩包条目默认分页大小
	defaultArchivePageSize = 200
	// maxArchivePageSize 压缩包条目单页上限
	maxArchivePageSize = 1000
	// maxArchiveEntries 单次压缩包扫描的条目上限
	maxArchiveEntries = 50000
)

// dbExtensions 资源库允许只读预览的数据库扩展名。
var dbExtensions = map[string]bool{
	".db": true, ".sqlite": true, ".sqlite3": true,
}

// 资源库变更库的文件位置（相对 LocalDir）。
const (
	// dbDirRel 数据库子目录
	dbDirRel = "database"
	// dbFileName 资源库变更库文件名
	dbFileName = "library.db"
	// legacyEditsFileRel 旧格式编辑状态 JSON（若存在则首次打开时导入）
	legacyEditsFileRel = "database/library_edits.json"
)

// 资源库变更库表名。
const (
	editTable       = "library_edit"
	fileStateTable  = "library_file_state"
	metaTable       = "library_meta"
	metaSeqKey      = "seq"
	editTableIndex  = "idx_library_edit_path_seq"
	stateTableIndex = "idx_library_file_state_seq"
)

// 文件视觉状态：前端据此着色。
const (
	// StatusNormal 无操作记录：系统默认配色
	StatusNormal = "normal"
	// StatusActive AI 最近查看 / 操作过：黄色
	StatusActive = "active"
	// StatusOK 运行完成无异常：绿色
	StatusOK = "ok"
	// StatusError 运行抛出异常：红色
	StatusError = "error"
)

// Roots 资源库根目录（句柄系统的 4 个存储根）。
var Roots = resource.StorageRoots

var (
	// dbMu 保护变更库连接与路径
	dbMu sync.Mutex
	// db 资源库变更库连接（数据库是变更记录的唯一真相来源）
	db *sql.DB
	// dbPath 当前连接对应的库路径（LocalDir 变化时自动重开）
	dbPath string

	// broadcastMu 保护广播句柄
	broadcastMu sync.RWMutex
	broadcast   func(msgType string, data any)

	// librarySchema 资源库变更库建表语句
	librarySchema = []string{
		`CREATE TABLE IF NOT EXISTS ` + editTable + ` (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			seq        INTEGER NOT NULL,
			path       TEXT    NOT NULL,
			op         TEXT    NOT NULL,
			added      TEXT    NOT NULL DEFAULT '',
			removed    TEXT    NOT NULL DEFAULT '',
			start_line INTEGER NOT NULL DEFAULT 0,
			lines      INTEGER NOT NULL DEFAULT 0,
			handle     TEXT    NOT NULL DEFAULT '',
			failed     INTEGER NOT NULL DEFAULT 0,
			at         INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ` + editTableIndex + ` ON ` + editTable + `(path, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_library_edit_seq ON ` + editTable + `(seq)`,
		`CREATE TABLE IF NOT EXISTS ` + fileStateTable + ` (
			path      TEXT    PRIMARY KEY,
			status    TEXT    NOT NULL DEFAULT 'normal',
			op        TEXT    NOT NULL DEFAULT '',
			failed    INTEGER NOT NULL DEFAULT 0,
			handle    TEXT    NOT NULL DEFAULT '',
			at        INTEGER NOT NULL DEFAULT 0,
			has_run   INTEGER NOT NULL DEFAULT 0,
			run_ok    INTEGER NOT NULL DEFAULT 0,
			last_seq  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS ` + stateTableIndex + ` ON ` + fileStateTable + `(last_seq)`,
		`CREATE TABLE IF NOT EXISTS ` + metaTable + ` (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
	}
)
