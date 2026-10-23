package console

// SetBuildVersion 注入构建版本号（宿主启动时调用一次，显示在「运行状态」屏）
func SetBuildVersion(version string) {
	buildVersionMu.Lock()
	buildVersion = version
	buildVersionMu.Unlock()
}

// BuildVersion 返回宿主注入的构建版本号（未注入时为空串）
func BuildVersion() string {
	buildVersionMu.RLock()
	defer buildVersionMu.RUnlock()
	return buildVersion
}

// GetSnapshot 返回最新运行状态快照
func GetSnapshot() Snapshot {
	statsMu.RLock()
	snap := latestStats
	statsMu.RUnlock()

	// 版本号由宿主注入，与采样周期无关，取快照时直接附带
	snap.BuildVersion = BuildVersion()
	return snap
}
