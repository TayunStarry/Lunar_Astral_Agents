package TuiSystem

import (
	"LunarSubsystem/TuiSystem/ltpxmarket"
	"LunarSubsystem/TuiSystem/updatepack"
)

// SetFrontendHooks 注入前端管理能力（宿主在 TUI 启动前调用）：
//   - lunarProbe / crystalProbe：在线探针（nil 表示未知状态）
//   - lunarRelaunch / crystalRelaunch：窗口唤起器（nil 表示不支持重启）
func SetFrontendHooks(lunarProbe, crystalProbe func() bool, lunarRelaunch, crystalRelaunch func() error) {
	frontendStatusMu.Lock()
	defer frontendStatusMu.Unlock()
	statusLunar = lunarProbe
	statusCrystal = crystalProbe
	relaunchLunar = lunarRelaunch
	relaunchCrystal = crystalRelaunch
}

// ==== 更新包配置注入 ====

var (
	// updatePackCfg 更新包功能配置（宿主从 lunar_config.json 读取注入）
	updatePackCfg *updatepack.Config
)

// SetUpdatePackConfig 注入更新包配置（nil 表示未配置，更新包屏会提示）
func SetUpdatePackConfig(cfg *updatepack.Config) {
	updatePackCfg = cfg
}

// GetUpdatePackConfig 读取更新包配置
func GetUpdatePackConfig() *updatepack.Config {
	return updatePackCfg
}

// ==== 应用市场配置注入 ====

var (
	// marketCfg 应用市场配置（宿主从 lunar_config.json 读取注入）
	marketCfg *ltpxmarket.Config
)

// SetMarketConfig 注入应用市场配置（nil 表示未配置，市场屏会提示）
func SetMarketConfig(cfg *ltpxmarket.Config) {
	marketCfg = cfg
}

// GetMarketConfig 读取应用市场配置
func GetMarketConfig() *ltpxmarket.Config {
	return marketCfg
}
