package lunar

import (
	"fmt"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// modelEnsureHook 模型服务保活钩子（宿主注入）
var modelEnsureHook atomic.Pointer[func() error]

// SetModelEnsureHook 注入模型服务保活动作
func SetModelEnsureHook(hook func() error) {
	if hook == nil {
		modelEnsureHook.Store(nil)
		return
	}
	modelEnsureHook.Store(&hook)
}

// EngineState 返回引擎状态快照
func EngineState() EngineStatus {
	engineMu.RLock()
	out := engineStatus
	engineMu.RUnlock()
	out.RuntimeReady = running.Load()
	out.AgentReady = IsReady()
	out.AutoReload = config.EngineAutoReload
	out.AutoReloadThreshold = config.EngineAutoReloadFailures
	return out
}

// ReloadRunning 是否有重载正在执行
func ReloadRunning() bool {
	return reloadRunning.Load()
}

// ReloadMoonEngine 重建对话链状态（Go 原生实现不再需要替换运行时）
func ReloadMoonEngine(reason string) error {
	if reason == "" {
		reason = "手动重载"
	}
	reloadRunning.Store(true)
	defer reloadRunning.Store(false)
	logger.Warn(lunarCoreModule, "重建月华对话链状态：%s", reason)

	if hook := modelEnsureHook.Load(); hook != nil {
		if err := (*hook)(); err != nil {
			logger.Error(lunarCoreModule, "模型服务保活失败: %v", err)
		}
	}

	stateMu.Lock()
	reasoningInProgress = false
	lastFailure = ""
	finalResponse = ""
	stateMu.Unlock()
	if dialogueRole != nil {
		dialogueRole.Messages = nil
		dialogueRole.RagMessages = nil
		dialogueRole.RuntimeMessages = nil
		dialogueRole.ToolRounds = 0
		dialogueRole.ActionClaimGuarded = false
	}
	restartTickScheduler()
	engineMu.Lock()
	engineStatus.ReloadCount++
	engineStatus.LastReloadAt = time.Now()
	engineStatus.LastReloadReason = reason
	engineStatus.LastReloadResult = "对话链状态已重建"
	engineStatus.BootedAt = time.Now()
	engineStatus.ConsecutiveFailures = 0
	engineMu.Unlock()
	return nil
}

// NotifyAgentFailure 上报本轮应答失败
func NotifyAgentFailure(reason string) {
	engineMu.Lock()
	engineStatus.ConsecutiveFailures++
	engineStatus.LastFailure = reason
	engineStatus.LastFailureAt = time.Now()
	engineMu.Unlock()
	logger.Warn(lunarCoreModule, "应答失败（连续 %d 次）: %s", EngineState().ConsecutiveFailures, reason)
}

// NotifyAgentIdle 上报本轮模型调用成功
func NotifyAgentIdle() {
	engineMu.Lock()
	engineStatus.ConsecutiveFailures = 0
	engineMu.Unlock()
}

func notifyAgentSuccess() {
	NotifyAgentIdle()
}

func reportAgentFailure(reason string) {
	NotifyAgentFailure(reason)
}

// ResetConversationState 重置对话状态（重载与测试用）
func ResetConversationState() {
	stateMu.Lock()
	reasoningInProgress = false
	fastAckSent = false
	turnFromQQ = false
	finalResponse = ""
	lastFailure = ""
	stateMu.Unlock()
	unreadMu.Lock()
	unreadContext = nil
	unreadRecords = nil
	unreadMu.Unlock()
}

var _ = fmt.Sprintf
