package logbuf

// version.go 构建版本号（宿主注入，供运行状态与自述文档读取）。

// SetBuildVersion 注入构建版本号（宿主启动时调用一次）。
func SetBuildVersion(version string) {
	buildVersionMu.Lock()
	buildVersion = version
	buildVersionMu.Unlock()
}

// BuildVersion 返回宿主注入的构建版本号（未注入时为空串）。
func BuildVersion() string {
	buildVersionMu.RLock()
	defer buildVersionMu.RUnlock()
	return buildVersion
}
