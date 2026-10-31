package LunarPersonality

// ==== 外部桌面智能体中断钩子 ====
// 桌面操纵直接操作真实桌面键鼠，仅允许在控制台人工启动。对话者一旦开始推理与应答，
// 必须立即终止它继续操作，避免与本轮响应、用户输入互相抢占鼠标键盘。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// SetDesktopActorStopHook 注入外部桌面智能体中断器（宿主在启动时调用；nil 表示不接入）。
// 中断器返回是否确有任务被中断，便于只在确实打断时记录日志。
func SetDesktopActorStopHook(hook func() bool) {
	if hook == nil {
		desktopActorStopHook.Store(nil)
		return
	}
	desktopActorStopHook.Store(&hook)
}

// stopDesktopActor 在对话者开始推理前终止外部桌面智能体（未注入时忽略）。
func stopDesktopActor() {
	hook := desktopActorStopHook.Load()
	if hook == nil {
		return
	}
	if (*hook)() {
		logger.Info(lunarCoreModule, "[对话者] 已终止外部桌面智能体，避免它继续操作桌面")
	}
}
