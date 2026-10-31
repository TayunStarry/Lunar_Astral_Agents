package broadcaster

// maintenance.go 播报链路的维护态：暂停响应并卸载在位 TTS 引擎
//
// 打包 / 压缩等重资源操作期间，播报链路若继续合成会拉起 TTS 模型，与 7z 抢内存
// 与 CPU。因此宿主在维护开始时调用 Pause：先置维护位拒绝新的合成与引擎加载，
// 再等待进行中的合成收尾（最长 maintenanceWait），最后卸载在位引擎。
// 维护结束后调用 Resume；模型不在维护期间被主动加载，下一次真实合成时按需加载。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Pause 进入维护态：拒绝新的合成请求与引擎加载，并卸载在位引擎（幂等）。
// 若进行中的合成在 maintenanceWait 内仍未收尾，则只保持拒绝响应、不强行卸载。
func Pause() {
	if !slotPaused.CompareAndSwap(false, true) {
		return
	}
	if !waitSlotIdle() {
		logger.Warn(logModule, "[维护] 当前合成仍在进行，暂不卸载在位引擎；维护期间不再响应新的合成请求")
		return
	}
	if err := Release(); err != nil {
		logger.Warn(logModule, "[维护] 卸载在位 TTS 引擎失败（继续）: %v", err)
		return
	}
	logger.Info(logModule, "[维护] 播报链路已暂停响应，在位 TTS 引擎已卸载")
}

// Resume 退出维护态：恢复合成与引擎加载（不预加载任何引擎）
func Resume() {
	if slotPaused.CompareAndSwap(true, false) {
		logger.Info(logModule, "[维护] 播报链路已恢复响应")
	}
}

// Paused 报告是否处于维护态
func Paused() bool {
	return slotPaused.Load()
}

// waitSlotIdle 等待槽位离开加载/合成/卸载中间态；超时返回 false
func waitSlotIdle() bool {
	deadline := time.Now().Add(maintenanceWait)
	for {
		if slotIdle() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(maintenancePoll)
	}
}

// slotIdle 槽位是否处于可安全卸载的空闲态（未加载且未合成）
func slotIdle() bool {
	switch slotStateVal.Load() {
	case stateUnloaded, stateReady:
		return true
	default:
		return false
	}
}
