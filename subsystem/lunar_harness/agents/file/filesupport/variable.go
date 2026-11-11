package filesupport

// variable.go 版本库的存储位置、保留策略与连接状态。

import (
	"database/sql"
	"sync"
	"sync/atomic"
)

const (
	// versionDBDirRel 版本库所在子目录（相对 LocalDir）
	versionDBDirRel = "database"
	// versionDBFileName 版本库文件名
	versionDBFileName = "versions.db"
	// moduleName 日志模块名
	moduleName = "文件版本"

	// legacyVersionRootRel 旧版按目录保存备份的根目录；首次打开时导入后改名
	legacyVersionRootRel = "file_versions"
	// migratedVersionRootRel 旧目录迁移后的名字（保留一份，确认无误后可手动删除）
	migratedVersionRootRel = "file_versions.migrated"

	// maxVersionsPerFile 单个文件最多保留的版本数
	maxVersionsPerFile = 50
	// maxTotalVersions 全库版本总数上限（超出按时间删除最旧）
	maxTotalVersions = 20000
	// maxVersionAgeDays 版本最长保留天数
	maxVersionAgeDays = 90
	// cleanupEveryWrites 每写入多少个版本执行一次全库保留策略
	cleanupEveryWrites = 64
	// maxVersionBlobBytes 单个版本内容上限；超过则拒绝备份，避免库被超大文件撑爆
	maxVersionBlobBytes = 64 << 20
)

// versionSchema 版本库建表语句。
var versionSchema = []string{
	`CREATE TABLE IF NOT EXISTS file_version (
		id          TEXT PRIMARY KEY,
		path        TEXT NOT NULL,
		op          TEXT NOT NULL DEFAULT '',
		exists_flag INTEGER NOT NULL DEFAULT 0,
		size        INTEGER NOT NULL DEFAULT 0,
		sha256      TEXT NOT NULL DEFAULT '',
		content     BLOB,
		created_at  INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_file_version_path ON file_version(path, created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_file_version_created ON file_version(created_at)`,
}

var (
	// versionMu 保护连接与迁移状态
	versionMu sync.Mutex
	// versionDB 版本库连接
	versionDB *sql.DB
	// versionDBPath 当前连接对应的库路径（LocalDir 变化时自动重开）
	versionDBPath string
)

// versionWrites 备份写入计数，用于控制全库清理频率。
var versionWrites atomic.Int64

// versionSeq 同一纳秒内的版本序号：避免连续写入落到同一个主键。
var versionSeq atomic.Int64
