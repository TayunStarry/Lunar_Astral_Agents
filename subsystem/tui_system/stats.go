package TuiSystem

// GetSnapshot 返回最新运行状态快照
func GetSnapshot() Snapshot {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return latestStats
}
