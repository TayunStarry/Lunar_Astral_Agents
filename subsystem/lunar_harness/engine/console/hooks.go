package console

import (
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/ltpxmarket"
	"LunarSubsystem/LunarHarness/engine/console/updatepack"
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

// ==== 月华对话链自愈钩子 ====
//
// 为什么要有这一屏：goja 运行时是一整块进程内状态（角色实例、对话上下文、思考中标记、
// 每秒 tick 的 setInterval）。它走进死路时——实测一次 llama-server 崩溃即可——
// 进程内没有任何自愈入口，用户只能重启整个程序（连带前端窗口与全部子系统）。
// 这里给它一个"最小代价的恢复手段"：手动重载 + 状态可见，自动重载则由宿主侧兜底。

// EngineStatus 月华对话链状态摘要（渲染数据源，由宿主注入的 status 钩子提供）
type EngineStatus struct {
	// RuntimeReady 对话链循环是否在运行
	RuntimeReady bool
	// AgentReady 智能体是否已就绪
	AgentReady bool
	// BootedAt 最近一次成功启动的时刻
	BootedAt time.Time
	// ReloadCount 累计重建次数
	ReloadCount int
	// LastReloadAt / LastReloadReason / LastReloadResult 最近一次重载的时刻、原因与结果
	LastReloadAt     time.Time
	LastReloadReason string
	LastReloadResult string
	// ConsecutiveFailures 连续应答失败次数；LastFailure 为最近一次失败原因
	ConsecutiveFailures int
	LastFailure         string
	LastFailureAt       time.Time
	// AutoReload / AutoReloadThreshold 自动重载策略
	AutoReload          bool
	AutoReloadThreshold int
	// Reloading 是否有重载正在执行
	Reloading bool
}

var (
	// engineHookMu 保护引擎自愈钩子
	engineHookMu sync.RWMutex
	// engineStatusHook 读取引擎状态
	engineStatusHook func() EngineStatus
	// engineReloadHook 触发一次重载（阻塞到重载完成）
	engineReloadHook func(reason string) error
)

// SetEngineHooks 注入引擎自愈能力（宿主在 TUI 启动前调用；nil 表示不支持）
func SetEngineHooks(status func() EngineStatus, reload func(reason string) error) {
	engineHookMu.Lock()
	defer engineHookMu.Unlock()
	engineStatusHook = status
	engineReloadHook = reload
}

// getEngineStatus 读取引擎状态；宿主未注入时返回空状态（界面会提示"宿主未接入"）
func getEngineStatus() (EngineStatus, bool) {
	engineHookMu.RLock()
	hook := engineStatusHook
	engineHookMu.RUnlock()
	if hook == nil {
		return EngineStatus{}, false
	}
	return hook(), true
}

// reloadMoonEngine 触发宿主重载引擎（未注入时返回 errNotWired）
func reloadMoonEngine(reason string) error {
	engineHookMu.RLock()
	hook := engineReloadHook
	engineHookMu.RUnlock()
	if hook == nil {
		return errNotWired
	}
	return hook(reason)
}

// ==== B 站真实浏览器生活钩子 ====
//
// TUI 只负责发起；真正的浏览器登录、选片、观看、评论、动态都在宿主 Go 模块里完成。
// 钩子返回一句可直接显示的状态说明；执行过程通过引擎日志屏展示。

var (
	biliLifeHookMu sync.RWMutex
	biliLifeHook   func() (string, error)
)

// SetBilibiliLifeHook 注入“开始刷视频”能力（宿主在 TUI 启动前调用；nil 表示不支持）
func SetBilibiliLifeHook(start func() (string, error)) {
	biliLifeHookMu.Lock()
	defer biliLifeHookMu.Unlock()
	biliLifeHook = start
}

// startBilibiliLife 请求宿主启动一次 B 站生活（未注入时返回 errNotWired）
func startBilibiliLife() (string, error) {
	biliLifeHookMu.RLock()
	hook := biliLifeHook
	biliLifeHookMu.RUnlock()
	if hook == nil {
		return "", errNotWired
	}
	return hook()
}
