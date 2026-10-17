package TuiSystem

import (
	"sync"

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

// ==== 数据导出钩子注入 ====

// DataDumpResult 一次数据导出的结果
type DataDumpResult struct {
	Path    string // 落盘路径（相对程序根目录，供用户查找）
	Bytes   int    // 文件字节数
	Records int    // 记录条数（日志行数 / 轮次数 / 上下文消息数）
}

var (
	// dumpHookMu 保护数据导出钩子
	dumpHookMu sync.RWMutex
	// logsDumpHook 原始日志导出（由宿主注入）
	logsDumpHook func() (DataDumpResult, error)
	// transcriptDumpHook 核心智能实况（模型调用轮次）导出（由宿主注入）
	transcriptDumpHook func() (DataDumpResult, error)
	// contextDumpHook 智能体上下文导出（由宿主注入；target 为 lunar 或子智能体名）
	contextDumpHook func(target string) (DataDumpResult, error)
)

// SetDataDumpHooks 注入数据导出能力（宿主在 TUI 启动前调用）：
//   - logs：导出原始日志缓冲为文件；
//   - transcript：导出核心智能实况的模型调用轮次为文件；
//   - context：按目标导出智能体上下文为文件。
//
// 三者都由宿主落盘：数据虽分别来自本包（日志缓冲 / 轮次缓冲）与月华 goja 运行时，
// 但**落盘路径与配置属于宿主**，且导出格式必须与对外端点共用同一份定义
// （宿主实现进程内直调端点处理器，避免"控制台导出的文件"与"端点拿到的文件"分叉）。
func SetDataDumpHooks(
	logs func() (DataDumpResult, error),
	transcript func() (DataDumpResult, error),
	context func(target string) (DataDumpResult, error),
) {
	dumpHookMu.Lock()
	defer dumpHookMu.Unlock()
	logsDumpHook = logs
	transcriptDumpHook = transcript
	contextDumpHook = context
}

// DumpLogs 导出原始日志（钩子未注入时返回 errNotWired）
func DumpLogs() (DataDumpResult, error) {
	dumpHookMu.RLock()
	hook := logsDumpHook
	dumpHookMu.RUnlock()
	if hook == nil {
		return DataDumpResult{}, errNotWired
	}
	return hook()
}

// DumpTranscript 导出核心智能实况的模型调用轮次（钩子未注入时返回 errNotWired）
func DumpTranscript() (DataDumpResult, error) {
	dumpHookMu.RLock()
	hook := transcriptDumpHook
	dumpHookMu.RUnlock()
	if hook == nil {
		return DataDumpResult{}, errNotWired
	}
	return hook()
}

// DumpContext 导出指定目标的智能体上下文（钩子未注入时返回 errNotWired）
func DumpContext(target string) (DataDumpResult, error) {
	dumpHookMu.RLock()
	hook := contextDumpHook
	dumpHookMu.RUnlock()
	if hook == nil {
		return DataDumpResult{}, errNotWired
	}
	return hook(target)
}
