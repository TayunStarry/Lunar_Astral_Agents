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

// ==== LTP9 授权签发钩子注入 ====
//
// 为什么由宿主实现：签发要「取 execute.js 的哈希 → 整体加密权限清单 → 覆盖写
// permissions.key」，这套规则必须与 LTP9 引擎的解码侧（engine/plugin/permission.go）
// 严格对应；两侧放在同一处实现（引擎侧）才不会出现「控制台签发的密钥引擎解不开」。
// 控制台只负责扫包、呈现授权状态与触发签发，不复制这套密码学规则。

// LTP9AuthStatus 单个 LTP9 包的授权现状（授权屏渲染数据源）
type LTP9AuthStatus struct {
	HasScript bool                // execute.js 是否存在（不存在即不可签发）
	HasKey    bool                // permissions.key 是否存在
	Signed    bool                // 现有密钥能否用当前脚本哈希解开（与运行时判据一致）
	Granted   []string            // 现有密钥解出的权限
	Required  []string            // 脚本现有引用推断出的权限
	Reasons   map[string][]string // 权限 → 命中的调用点（确认页呈现签发依据）
	Hash      string              // execute.js 的 sha16
}

var (
	// ltp9AuthMu 保护 LTP9 授权钩子的并发读写
	ltp9AuthMu sync.RWMutex
	// ltp9AuthInspectHook 读取授权现状（不修改文件）
	ltp9AuthInspectHook func(dir string) (LTP9AuthStatus, error)
	// ltp9AuthSignHook 依据脚本现有引用签发（覆盖写 permissions.key）
	ltp9AuthSignHook func(dir string) (LTP9AuthStatus, error)
)

// SetLTP9AuthHooks 注入 LTP9 授权能力（宿主在 TUI 启动前调用）：
//   - inspect：读取某包的授权现状（不修改文件，供列表着色与依据展示）；
//   - sign：依据脚本现有引用签发授权并返回签发后的现状。
func SetLTP9AuthHooks(
	inspect func(dir string) (LTP9AuthStatus, error),
	sign func(dir string) (LTP9AuthStatus, error),
) {
	ltp9AuthMu.Lock()
	defer ltp9AuthMu.Unlock()
	ltp9AuthInspectHook = inspect
	ltp9AuthSignHook = sign
}

// InspectLTP9Auth 读取指定 LTP9 包的授权现状（钩子未注入时返回 errNotWired）
func InspectLTP9Auth(dir string) (LTP9AuthStatus, error) {
	ltp9AuthMu.RLock()
	hook := ltp9AuthInspectHook
	ltp9AuthMu.RUnlock()
	if hook == nil {
		return LTP9AuthStatus{}, errNotWired
	}
	return hook(dir)
}

// SignLTP9Auth 为指定 LTP9 包签发授权（钩子未注入时返回 errNotWired）
func SignLTP9Auth(dir string) (LTP9AuthStatus, error) {
	ltp9AuthMu.RLock()
	hook := ltp9AuthSignHook
	ltp9AuthMu.RUnlock()
	if hook == nil {
		return LTP9AuthStatus{}, errNotWired
	}
	return hook(dir)
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
