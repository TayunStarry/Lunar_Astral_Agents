package dbopen

// variable.go opener 默认参数常量。

const (
	// DefaultBusyTimeoutMS 默认忙等超时（毫秒），与仓库既有 SQLite 库一致。
	DefaultBusyTimeoutMS = 10000
	// DefaultJournalMode 默认日志模式。
	DefaultJournalMode = "WAL"
)
