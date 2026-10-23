package lunarloop

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/lunar_loop/container"

	"github.com/dop251/goja"
)

// ==== 月华引擎（goja lunar_loop）重载与自愈 ====
//
// 为什么需要这套东西：JS 运行时是一整块**进程内内存状态**——角色实例、对话上下文、
// 思考中标记、每秒一次的 setInterval。它一旦走进死路，进程里没有任何东西能把它救回来：
//
//   - 实测事故：llama-server 因内部断言崩溃（`GGML_ASSERT(cparams.causal_attn || ...)`）后，
//     模型请求全部"Could not establish connection"；JS 侧 try/catch 吞掉异常，后续
//     `reasoningInProgress` 因提前 return 没复位，每秒的 tick 在入口就 return，
//     **整个思考链永久停摆**，而界面上没有任何错误提示。
//   - 另一类：上下文被夹断（assistant 有 tool_calls 无配对结果）后，模型每轮复读同一次调用，
//     越走越偏；这种"逻辑坏死"同样只能靠换一块干净的运行时脱身。
//
// 取舍（用户明示）：**维持思维链 > 保住上下文**。因此重载优先保证"能继续想"，
// 上下文只是尽力抢救（图片一律省略，见 agent/recovery.ts）。

// reloadCooldown 自动重载的冷却时间：太频繁的重载比停摆更糟（每轮都在丢上下文）
const reloadCooldown = 60 * time.Second

// EngineStatus 引擎自愈状态（TUI「月华引擎」屏与日志用）
type EngineStatus struct {
	// RuntimeReady goja 运行时是否存在
	RuntimeReady bool
	// AgentReady agentSystem.js 是否已成功加载
	AgentReady bool
	// BootedAt 最近一次成功启动的时刻（零值表示尚未启动）
	BootedAt time.Time
	// ReloadCount 累计重载次数
	ReloadCount int
	// LastReloadAt / LastReloadReason / LastReloadResult 最近一次重载的时刻、原因、结果
	LastReloadAt     time.Time
	LastReloadReason string
	LastReloadResult string
	// ConsecutiveFailures 连续应答失败次数（成功一次即清零）
	ConsecutiveFailures int
	// LastFailure 最近一次失败原因
	LastFailure   string
	LastFailureAt time.Time
	// AutoReload / AutoReloadThreshold 当前生效的自动重载策略
	AutoReload          bool
	AutoReloadThreshold int
}

var (
	// reloadMu 串行化重载（同一时刻只允许一次；自动重载用 TryLock 抢）
	reloadMu sync.Mutex

	// statusMu 保护 engineStatus
	statusMu sync.RWMutex
	// engineStatus 自愈状态
	engineStatus = EngineStatus{}

	// snapshotMu 保护 pendingSnapshot（重载前暂存，新运行时启动后回灌）
	snapshotMu      sync.Mutex
	pendingSnapshot string

	// modelEnsureHook 模型服务保活钩子（宿主注入；由 app 层绑到 llama 包，
	// 避免 JS 运行时直接依赖模型代理的实现细节）
	modelEnsureHook atomic.Pointer[func() error]

	// reloadRunning 是否有重载正在执行（TUI 据此显示"进行中"）
	reloadRunning atomic.Bool
)

// SetModelEnsureHook 注入模型服务保活动作（宿主在启动 TUI 前调用；nil 表示不检查）
//
// 返回 nil 表示模型服务可用；非 nil 表示仍不可用。重载引擎时若服务已崩，
// 必须先把它拉起来——否则新运行时起来也连不上，只会继续失败。
func SetModelEnsureHook(hook func() error) {
	if hook == nil {
		modelEnsureHook.Store(nil)
		return
	}
	modelEnsureHook.Store(&hook)
}

// EngineState 返回引擎自愈状态快照（同时反映运行时与智能体的实时就绪情况）
func EngineState() EngineStatus {
	statusMu.RLock()
	out := engineStatus
	statusMu.RUnlock()
	out.RuntimeReady = container.IsReady()
	out.AgentReady = IsAgentReady()
	out.AutoReload = *config.EngineAutoReload
	out.AutoReloadThreshold = *config.EngineAutoReloadFailures
	return out
}

// ReloadRunning 是否有重载正在执行（TUI 用它避免重复触发、显示进度）
func ReloadRunning() bool {
	return reloadRunning.Load()
}

// ReloadMoonEngine 重载月华引擎：抢救上下文 → 换一块干净的 goja 运行时 → 回灌上下文
//
// 顺序不能变：快照必须在旧运行时还活着时取（关掉之后就取不到了），
// 而回灌必须在新运行时把 agentSystem.js 跑完之后（否则没有角色实例可写）。
func ReloadMoonEngine(reason string) error {
	reloadMu.Lock()
	defer reloadMu.Unlock()
	reloadRunning.Store(true)
	defer reloadRunning.Store(false)

	if reason == "" {
		reason = "手动重载"
	}
	logger.Warn(lunarCoreModule, "重载月华引擎（goja 运行时）：%s", reason)

	// 1) 抢救上下文（失败不阻断：用户在优先级上已选"先能继续想"）
	snapshot := ""
	if IsAgentReady() {
		text, err := container.EvalString("agentSystem.snapshotContext()")
		if err != nil {
			logger.SubWarn(lunarCoreModule, lunarJavaScriptSub, "重载前取上下文快照失败: %v", err)
		} else {
			snapshot = text
		}
	}
	setPendingSnapshot(snapshot)

	// 2) 关闭旧运行时（旧 VM 连同其中的上下文、计时器一起作废）
	container.Close()
	agentReady.Store(false)

	// 3) 模型服务保活：服务崩了，重载 goja 也连不上（实测正是这种事故）
	modelNote := ensureModelServer()

	// 4) 启动新运行时
	if err := ensureRuntime(); err != nil {
		recordReload(reason, false, "创建运行时失败: "+err.Error())
		return err
	}
	if err := runAgentScript(); err != nil {
		recordReload(reason, false, "加载 agentSystem.js 失败: "+err.Error())
		return err
	}

	// 5) 回灌上下文（快照为空或版本不符时只记录，不影响引擎可用）
	restoreNote := restoreSnapshot()
	result := fmt.Sprintf("运行时就绪；%s；上下文：%s", modelNote, restoreNote)
	recordReload(reason, true, result)
	logger.Info(lunarCoreModule, "月华引擎已重载：%s", result)
	return nil
}

// ensureModelServer 确保模型服务可用；返回一行可读结论（写进重载结果）
func ensureModelServer() string {
	hook := modelEnsureHook.Load()
	if hook == nil {
		return "模型服务未接入保活检查"
	}
	if err := (*hook)(); err != nil {
		logger.Error(lunarCoreModule, "模型服务保活失败: %v", err)
		return "模型服务仍不可用（" + err.Error() + "）"
	}
	return "模型服务可用"
}

// setPendingSnapshot 暂存重载前的上下文快照
func setPendingSnapshot(text string) {
	snapshotMu.Lock()
	pendingSnapshot = text
	snapshotMu.Unlock()
}

// takePendingSnapshot 取出（并清空）重载前的上下文快照
func takePendingSnapshot() string {
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	out := pendingSnapshot
	pendingSnapshot = ""
	return out
}

// restoreSnapshot 在新运行时里回灌上下文；返回一行可读结论
func restoreSnapshot() string {
	note, err := container.EvalString("agentSystem.restoreContext(takeEngineSnapshot())")
	if err != nil {
		// 全新运行时理应带着恢复入口；这里失败说明 agentSystem.js 与宿主版本不匹配
		return "恢复入口不可用: " + err.Error()
	}
	if note == "" {
		return "无快照可恢复"
	}
	return note
}

// recordReload 记录一次重载结果
func recordReload(reason string, ok bool, result string) {
	statusMu.Lock()
	engineStatus.ReloadCount++
	engineStatus.LastReloadAt = time.Now()
	engineStatus.LastReloadReason = reason
	engineStatus.LastReloadResult = result
	if ok {
		engineStatus.BootedAt = time.Now()
		// 重载成功意味着"从零开始"：连续失败计数清零，避免刚修好又被判死
		engineStatus.ConsecutiveFailures = 0
	}
	statusMu.Unlock()
}

// NotifyAgentFailure JS 上报：本轮应答失败
//
// 连续失败达到阈值且不在冷却期内 → 后台重载引擎。为什么要"连续"而不是"一次"：
// 一次网络抖动完全可能在下一轮自愈，立刻重载等于白白丢掉上下文。
func NotifyAgentFailure(reason string) {
	statusMu.Lock()
	engineStatus.ConsecutiveFailures++
	engineStatus.LastFailure = reason
	engineStatus.LastFailureAt = time.Now()
	failures := engineStatus.ConsecutiveFailures
	lastReload := engineStatus.LastReloadAt
	statusMu.Unlock()

	logger.SubWarn(lunarCoreModule, lunarJavaScriptSub,
		"应答失败（连续 %d 次）: %s", failures, reason)

	if !*config.EngineAutoReload || *config.EngineAutoReloadFailures <= 0 {
		return
	}
	if failures < *config.EngineAutoReloadFailures {
		return
	}
	// 冷却期：重载本身会丢上下文，连续失败的密集区里反复重载只会更糟
	if !lastReload.IsZero() && time.Since(lastReload) < reloadCooldown {
		logger.SubWarn(lunarCoreModule, lunarJavaScriptSub,
			"距上次重载不足 %s，跳过本次自动重载", reloadCooldown)
		return
	}
	// 已有重载在跑：让它的结果说话，不要再排队
	if !reloadMu.TryLock() {
		return
	}
	reloadMu.Unlock()

	logger.Warn(lunarCoreModule, "连续 %d 次应答失败，自动重载月华引擎", failures)
	go func() {
		if err := ReloadMoonEngine(fmt.Sprintf("连续 %d 次应答失败：%s", failures, reason)); err != nil {
			logger.Error(lunarCoreModule, "自动重载月华引擎失败: %v", err)
		}
	}()
}

// NotifyAgentIdle JS 上报：本轮模型调用成功（清零连续失败计数）
func NotifyAgentIdle() {
	statusMu.Lock()
	engineStatus.ConsecutiveFailures = 0
	statusMu.Unlock()
}

// ==== goja 绑定：JS → 宿主的自愈上报 ====

// notifyAgentIdle 绑定：notifyAgentIdle()
func (class *Runtime) notifyAgentIdle(goja.FunctionCall) goja.Value {
	NotifyAgentIdle()
	return goja.Undefined()
}

// notifyAgentFailure 绑定：notifyAgentFailure(reason)
func (class *Runtime) notifyAgentFailure(call goja.FunctionCall) goja.Value {
	NotifyAgentFailure(call.Argument(0).String())
	return goja.Undefined()
}

// takeEngineSnapshot 绑定：takeEngineSnapshot() → 重载前的上下文快照（一次性）
func (class *Runtime) takeEngineSnapshot(goja.FunctionCall) goja.Value {
	return class.runtime.ToValue(takePendingSnapshot())
}
