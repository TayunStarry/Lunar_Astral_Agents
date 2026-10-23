package config

// ==== 台词链自愈策略 ====

var (
	// EngineAutoReload 应答连续失败达阈值时，是否自动重建月华台词链状态
	EngineAutoReload = true
	// EngineAutoReloadFailures 连续失败多少次触发自动重建
	EngineAutoReloadFailures = 2
)
