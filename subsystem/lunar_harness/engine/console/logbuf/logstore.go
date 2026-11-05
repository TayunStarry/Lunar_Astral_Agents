package logbuf

// logstore.go 日志环形缓冲：写入、读取、清空。不依赖 TUI 的唤醒机制，
// 视图刷新信号由 console 根包在包装层自行触发。

// PushLog 追加一条日志到指定来源缓冲（环形截断）。
func PushLog(line LogLine, source LogSource) {
	storeMu.Lock()
	store := &logStores[source]
	*store = append(*store, line)
	if len(*store) > LogStoreCap {
		*store = (*store)[len(*store)-LogStoreCap:]
	}
	// 版本号必须每次追加都递增：缓冲满后 len 恒为 LogStoreCap，
	// 只靠长度做缓存失效会让日志屏停在旧内容上不再刷新。
	logStoreVersions[source]++
	storeMu.Unlock()
}

// GetLogsWithVersion 返回日志缓冲副本与追加版本号。
// 视图缓存必须以版本号（而非 len(logs)）失效：环形缓冲满后长度不再变化。
func GetLogsWithVersion(source LogSource) ([]LogLine, uint64) {
	storeMu.RLock()
	defer storeMu.RUnlock()
	out := make([]LogLine, len(logStores[source]))
	copy(out, logStores[source])
	return out, logStoreVersions[source]
}

// GetLogs 返回指定来源的日志缓冲副本（视图渲染 / 导出数据源）。
func GetLogs(source LogSource) []LogLine {
	logs, _ := GetLogsWithVersion(source)
	return logs
}

// ClearLogs 清空指定来源的日志缓冲。
func ClearLogs(source LogSource) {
	storeMu.Lock()
	logStores[source] = nil
	logStoreVersions[source]++
	storeMu.Unlock()
}
