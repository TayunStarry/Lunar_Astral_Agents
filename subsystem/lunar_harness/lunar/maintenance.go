package lunar

// maintenance.go 重资源操作期间的思维链暂停与模型卸载
//
// 打包 / 压缩等重磁盘与 CPU 操作期间，Tick 若继续推进生活链或对话链，
// 会突然拉起 primary_reasoning / light_reasoning / text_embedding，与 7z 抢资源。
// 因此由控制台在打包前调用 BeginMaintenance：先暂停 Tick、等待当前一轮结束，
// 再定向卸载三个对话模型；打包结束后 Resume。

import (
	"strings"
	"time"

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

// BeginMaintenance 进入维护态：暂停 Tick 并卸载对话模型，返回恢复函数。
//
// 若当前 Tick 在 30 秒内仍未结束，则不强行卸载（避免打断正在进行的模型调用），
// 但仍保持 Tick 暂停——打包期间不会再有新的思维链被拉起。
func BeginMaintenance() func() {
	paused := false
	if tickSchedulerInstance != nil {
		tickSchedulerInstance.Pause()
		paused = true
		if !tickSchedulerInstance.WaitIdle(maintenanceIdleWait) {
			logger.Warn(tickModule, "[维护] 当前思维链仍在运行，打包期间已暂停后续 Tick；本轮结束后才可卸载模型")
			return func() { tickSchedulerInstance.Resume() }
		}
	}
	unloadDialogModels()
	logger.Info(tickModule, "[维护] 已暂停思维链 Tick 并卸载对话模型")
	return func() {
		if paused && tickSchedulerInstance != nil {
			tickSchedulerInstance.Resume()
		}
		// 只恢复 Tick 调度：不主动加载任何模型，下一次真正需要推理时由 router 按需加载
		logger.Info(tickModule, "[维护] 维护结束，Tick 调度已恢复")
	}
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
