package LunarPersonality

// maintenance.go 重资源操作期间的思维链暂停与模型卸载
//
// 打包 / 压缩等重磁盘与 CPU 操作期间，Tick 若继续推进生活链或对话链，
// 会突然拉起 primary_reasoning / light_reasoning / text_embedding，与 7z 抢资源。
// 因此由控制台在打包前调用 BeginMaintenance：先暂停 Tick、等待当前一轮结束，
// 再定向卸载三个对话模型；打包结束后 Resume。

import (
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/engine/broadcaster"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/model/llama"
)

const (
	// maintenanceIdleWait 等待当前 Tick 结束的最长时间
	maintenanceIdleWait = 30 * time.Second
	// maintenanceIdlePoll 等待轮询间隔
	maintenanceIdlePoll = 200 * time.Millisecond
)

// BeginMaintenance 进入维护态：同时暂停主调度 Tick 与响应者 Tick，暂停播报链路并
// 卸载在位 TTS 引擎，最后卸载对话模型，返回恢复函数。
//
// 若当前思维链或通用应答在 30 秒内仍未结束，则不强行卸载（避免打断正在进行的
// 模型调用），但两个 Tick 都保持暂停——打包期间不会再有新的模型调用被拉起；
// 播报链路同样保持拒绝响应，TTS 引擎的卸载由 broadcaster.Pause 自行等待收尾。
func BeginMaintenance() func() {
	// 响应者 Tick 独立于主调度器：先暂停它，确保 LTPX 特许运行也不会绕开维护态
	Responder.Pause()
	schedulerPaused := false
	if tickSchedulerInstance != nil {
		tickSchedulerInstance.Pause()
		schedulerPaused = true
	}
	// 播报链路：拒绝新的合成请求，并在已有合成收尾后卸载在位 TTS 引擎
	broadcaster.Pause()

	deadline := time.Now().Add(maintenanceIdleWait)
	schedulerIdle := tickSchedulerInstance == nil || tickSchedulerInstance.WaitIdle(time.Until(deadline))
	responderIdle := waitResponderIdle(deadline)
	if !schedulerIdle || !responderIdle {
		logger.Warn(tickModule, "[维护] 思维链或通用应答仍在运行，维护期间已暂停两个 Tick 与播报链路；当前调用结束后才可卸载模型")
		return func() { resumeMaintenanceTicks(schedulerPaused) }
	}

	unloadDialogModels()
	logger.Info(tickModule, "[维护] 已暂停思维链 Tick 与通用应答 Tick，并卸载对话模型与 TTS 引擎")
	return func() {
		resumeMaintenanceTicks(schedulerPaused)
		// 只恢复 Tick 调度与播报响应：不主动加载任何模型，下一次真正需要推理/合成时按需加载
		logger.Info(tickModule, "[维护] 维护结束，Tick 调度与播报链路已恢复")
	}
}

// waitResponderIdle 等待响应者当前一轮推理结束；超时返回 false。
func waitResponderIdle(deadline time.Time) bool {
	for {
		if !Responder.Busy() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(maintenanceIdlePoll)
	}
}

// resumeMaintenanceTicks 恢复两个 Tick 与播报链路（幂等，只恢复本次暂停过的部分）。
func resumeMaintenanceTicks(schedulerPaused bool) {
	Responder.Resume()
	if schedulerPaused && tickSchedulerInstance != nil {
		tickSchedulerInstance.Resume()
	}
	broadcaster.Resume()
}

// unloadDialogModels 定向卸载三个对话相关模型（幂等：未加载时为空操作）
//
// 同时覆盖配置里的模型名与项目内固定 ID：配置若指向别的模型，
// primary_reasoning / light_reasoning / text_embedding 仍可能被其它链路使用，必须一并卸载。
func unloadDialogModels() {
	models := []string{
		config.AgentMultimodalModel, "primary_reasoning",
		config.WebSearchModel, "light_reasoning",
		config.AgentEmbeddingModel, "text_embedding",
	}
	seen := map[string]bool{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		if _, err := llama.UnloadModelByID(model); err != nil {
			logger.Warn(tickModule, "[维护] 卸载模型[%s]失败（继续）: %v", model, err)
			continue
		}
		logger.Info(tickModule, "[维护] 已卸载模型[%s]", model)
	}
}
