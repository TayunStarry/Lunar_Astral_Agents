package console

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/ltpxmarket"
	"LunarSubsystem/LunarHarness/engine/console/updatepack"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
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
	// updatePackCfg 更新包功能配置（宿主从 local_data/release.json 读取注入）
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
	// marketCfg 应用市场配置（宿主从 local_data/release.json 读取注入）
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

// ==== 文件运行入口（「编辑显示」屏） ====

var (
	// fileRunMu 保护文件运行钩子
	fileRunMu sync.RWMutex
	// fileRunHook 由宿主注入：按相对 documents 的路径运行项目文件，返回操作简述
	fileRunHook func(rel string) (string, error)
)

// SetFileRunHook 注入「运行当前文件」能力（宿主在 TUI 启动前调用；nil 表示不支持）。
func SetFileRunHook(hook func(rel string) (string, error)) {
	fileRunMu.Lock()
	defer fileRunMu.Unlock()
	fileRunHook = hook
}

// RunFile 运行指定文档（rel 为相对 documents 的路径），返回操作简述。
func RunFile(rel string) (string, error) {
	fileRunMu.RLock()
	hook := fileRunHook
	fileRunMu.RUnlock()
	if hook == nil {
		return "", errNotWired
	}
	return hook(rel)
}

// ==== Go 智能体工具目录 ====

var (
	// toolCatalogMu 保护工具目录钩子
	toolCatalogMu sync.RWMutex
	// toolCatalogHook 由宿主汇总全部 Go 智能体的工具清单
	toolCatalogHook func() []agentinfo.Agent
)

// SetToolCatalogHook 注入 Go 智能体工具目录（宿主在 TUI 启动前调用）。
func SetToolCatalogHook(hook func() []agentinfo.Agent) {
	toolCatalogMu.Lock()
	defer toolCatalogMu.Unlock()
	toolCatalogHook = hook
}

// getToolCatalog 读取工具目录；未注入时返回 nil，由界面明确提示。
func getToolCatalog() []agentinfo.Agent {
	toolCatalogMu.RLock()
	hook := toolCatalogHook
	toolCatalogMu.RUnlock()
	if hook == nil {
		return nil
	}
	return hook()
}

// ==== 桌面操纵钩子 ====
//
// 桌面操纵直接操作真实桌面键鼠，存在安全风险：不从月华 LTPX 工具链暴露，
// 只能在「运行状态」屏由人工输入要求后启动，并支持随时中断。

// DesktopOperatorStatus 桌面操纵运行状态摘要（渲染数据源，由宿主注入的 status 钩子提供）
type DesktopOperatorStatus struct {
	Running     bool      // 是否正在运行
	Stopping    bool      // 是否已请求中断、正在等待退出
	Instruction string    // 本轮指令
	StartedAt   time.Time // 启动时间
	FinishedAt  time.Time // 结束时间（未结束时为零值）
	Result      string    // 成功时的结果文本
	Error       string    // 失败/中断原因（空串表示无）
}

var (
	// desktopOperatorHookMu 保护桌面操纵控制钩子
	desktopOperatorHookMu sync.RWMutex
	// desktopOperatorStartHook 启动一次任务
	desktopOperatorStartHook func(instruction string) error
	// desktopOperatorStopHook 中断当前任务（返回是否确有任务在运行）
	desktopOperatorStopHook func() bool
	// desktopOperatorStatusHook 读取运行状态
	desktopOperatorStatusHook func() DesktopOperatorStatus
)

// SetDesktopOperatorHooks 注入桌面操纵控制能力（宿主在 TUI 启动前调用；nil 表示不支持）
// stop 返回是否确有任务被中断，便于调用方决定是否提示。
func SetDesktopOperatorHooks(start func(instruction string) error, stop func() bool, status func() DesktopOperatorStatus) {
	desktopOperatorHookMu.Lock()
	defer desktopOperatorHookMu.Unlock()
	desktopOperatorStartHook = start
	desktopOperatorStopHook = stop
	desktopOperatorStatusHook = status
}

// startDesktopOperator 启动桌面操纵（未注入时返回 errNotWired）
func startDesktopOperator(instruction string) error {
	desktopOperatorHookMu.RLock()
	hook := desktopOperatorStartHook
	desktopOperatorHookMu.RUnlock()
	if hook == nil {
		return errNotWired
	}
	return hook(instruction)
}

// stopDesktopOperator 中断桌面操纵并返回是否确有任务在运行（未注入时返回 false）
func stopDesktopOperator() bool {
	desktopOperatorHookMu.RLock()
	hook := desktopOperatorStopHook
	desktopOperatorHookMu.RUnlock()
	if hook == nil {
		return false
	}
	return hook()
}

// getDesktopOperatorStatus 读取桌面操纵状态；宿主未注入时返回空状态
func getDesktopOperatorStatus() (DesktopOperatorStatus, bool) {
	desktopOperatorHookMu.RLock()
	hook := desktopOperatorStatusHook
	desktopOperatorHookMu.RUnlock()
	if hook == nil {
		return DesktopOperatorStatus{}, false
	}
	return hook(), true
}
