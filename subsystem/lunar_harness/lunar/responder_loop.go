package lunar

// responder_loop.go 响应者独立 Tick 循环（每秒一次，与主调度器并行）。
//
// 规则：
//  1. 对话 / 后勤 / 观影 / 生活任一体系运行中（或维护态）→ 本 Tick 直接跳过，
//     不发起任何模型操作，从而只在其他系统休整时响应外部推理请求；
//  2. 对话者调用 LTPX 工具期间处于特许运行状态（Responder.BeginPermit），
//     此时对话者阻塞在工具等待上、模型空闲，本 Tick 可以越过第 1 条运行；
//     该状态一直持续到对话者拿到工具返回，响应者 Tick 不清除它。

import (
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
)

// runResponderLoop 响应者 Tick 主循环
func runResponderLoop() {
	defer loopWG.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			responderTick()
		}
	}
}

// responderTick 一次响应者 Tick。
func responderTick() {
	scheduler := tickSchedulerInstance
	if scheduler == nil {
		return
	}
	// 特许运行状态、主调度空档期，任一成立即可运行
	if !Responder.Permitted() && !scheduler.Available() {
		return
	}
	Responder.Drain(responderCtx)
}
