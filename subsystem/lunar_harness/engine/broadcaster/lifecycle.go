package broadcaster

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 槽位闲置卸载守护 ====
//
// 单载槽位的回收机制：在位引擎超过阈值无合成调用即卸载模型占用，空槽在
// 下一次合成/专属端点请求时由槽位对齐逻辑自动重新加载。阈值由
// tts.idle_unload_minutes 配置（默认 5 分钟）。
//
// 「最近活动」以槽位为准（slotLastUseUnix：引擎加载成功或合成完成时更新），
// status/voices 等观测端点不触碰计时，health 类轮询不会阻止闲置回收。

// StartIdleUnloader 启动槽位闲置卸载守护（进程级单次启动，随 Assemble 调用）
func StartIdleUnloader() {
	// 阈值：启动时读一次配置；非法值（<=0）回退默认 5 分钟
	go func() {
		ticker := time.NewTicker(idleScanInterval * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanIdleSlot()
		}
	}()
	logger.Info(logModule, "槽位闲置卸载守护已启动（%d 分钟无合成调用即卸载在位引擎）", idleUnloadMinutes())
}

// idleUnloadMinutes 闲置卸载阈值（分钟）：非法配置回退默认值
func idleUnloadMinutes() int {
	if *config.TTSIdleUnloadMinutes > 0 {
		return *config.TTSIdleUnloadMinutes
	}
	return idleUnloadDefaultSeconds / 60
}

// scanIdleSlot 单轮闲置扫描：在位引擎超阈值即卸载（空槽跳过）
func scanIdleSlot() {
	if currentEngine() == "" {
		return
	}
	last := slotLastUseUnix.Load()
	if last == 0 {
		return
	}
	if nowUnix()-last < int64(idleUnloadMinutes())*60 {
		return
	}
	if err := Release(); err != nil {
		logger.Error(logModule, "槽位闲置卸载失败: %v", err)
		return
	}
	logger.Info(logModule, "在位引擎已闲置 %d 秒，卸载释放（下次调用时重新加载）", nowUnix()-last)
}
