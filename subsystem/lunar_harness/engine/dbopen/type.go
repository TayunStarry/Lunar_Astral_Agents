package dbopen

// type.go 统一 SQLite opener 的选项类型与错误类型。

import "time"

// Options SQLite 打开选项。
//
// 零值等价于仓库最常用组合：busy_timeout=10s + journal_mode=WAL。
// 连接池上限、连接存活时间与父目录创建默认不生效，由调用方按站点语义显式声明。
type Options struct {
	// BusyTimeoutMS 忙等超时（毫秒）；0 取默认 10000。
	BusyTimeoutMS int
	// JournalMode 日志模式；空取默认 "WAL"。
	JournalMode string
	// ForeignKeys 为 true 时在 DSN 中追加 _foreign_keys=on。
	ForeignKeys bool
	// MaxOpenConns >0 时设置最大打开连接数；0 表示不设置。
	MaxOpenConns int
	// MaxIdleConns >0 时设置最大空闲连接数；0 表示不设置。
	MaxIdleConns int
	// ConnMaxLifetime >0 时设置连接最长存活时间；0 表示不设置。
	ConnMaxLifetime time.Duration
	// CreateDir 为 true 时在打开前自动创建库文件父目录（权限 0755）。
	CreateDir bool
	// NoDSNParams 为 true 时完全不写 DSN 参数，忽略 BusyTimeoutMS / JournalMode / ForeignKeys。
	// 仅用于历史上裸开库、后续自行执行 PRAGMA 的站点。
	NoDSNParams bool
}

// DirError 父目录创建失败。调用方可用 DirCause 取回底层错误并保留自己的文案。
type DirError struct {
	Err error
}

// Error 返回底层错误文本。
func (e *DirError) Error() string { return e.Err.Error() }

// Unwrap 暴露底层错误，便于 errors.Is / errors.As 继续判断。
func (e *DirError) Unwrap() error { return e.Err }
