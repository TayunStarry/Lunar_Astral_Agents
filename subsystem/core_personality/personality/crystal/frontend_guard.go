package crystal

// LTPX 前端守护：琉璃前端离线时的自愈唤起。
//
// 背景：月华的机制运行不依赖前端（前端仅承担显示与记录），但琉璃与 LTPX 的功能
// 实现依托前端——ltpx_call 派发与 LTP9 插件沙箱都在琉璃桌面页面里执行。用户关掉
// 琉璃窗口后，月华发起的 LTPX 工具调用会广播给无人接收，直至轮询超时。
//
// 机制：前端包工具派发前经 EnsureFrontendForDispatch 检查就绪探针（缺省频道上的
// 总线订阅者数 > 0，即琉璃桌面以 /ws?bus=1 接入）；不在线则调用唤起器重建桌面
// 窗口，并轮询等待其重新接入总线（单飞 + 超时兜底，超时后按旧机制继续广播）。
// 唤起器与探针由装配层注入（app → crystal 单向依赖，避免反向 import）。

import (
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// SetFrontendRelauncher 注入琉璃前端唤起器（重建桌面窗口；窗口已存在时聚焦）
func SetFrontendRelauncher(fn func() error) {
	frontendRelauncher = fn
}

// SetFrontendReadyProbe 注入琉璃前端就绪探针（缺省频道存在总线订阅者即视为在线）
func SetFrontendReadyProbe(fn func() bool) {
	frontendReadyProbe = fn
}

// EnsureFrontendForDispatch 确保 LTPX 前端派发通道可用：琉璃前端离线时唤起其桌面
// 窗口并等待重新就绪。单飞保护：并发派发共享同一次唤起；等待超时后放行（回退旧
// 语义：广播无人接收，由轮询超时收尾），绝不无限阻塞派发 goroutine。
func EnsureFrontendForDispatch() {
	if frontendReadyProbe == nil {
		return
	}
	if frontendReadyProbe() {
		return
	}

	frontendGuardMu.Lock()
	defer frontendGuardMu.Unlock()

	// 双检：等锁期间前一次唤起可能已把前端拉起
	if frontendReadyProbe() {
		return
	}

	if frontendRelauncher == nil {
		return
	}
	LoggerGeneral.Info("CrystalAstral", "琉璃前端不在线（LTPX 功能依托前端），尝试重新唤起琉璃桌面窗口")
	if err := frontendRelauncher(); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "琉璃桌面窗口唤起失败: %v（本次派发按旧机制继续）", err)
	}

	deadline := time.Now().Add(frontendReviveTimeout)
	for time.Now().Before(deadline) {
		if frontendReadyProbe() {
			LoggerGeneral.Info("CrystalAstral", "琉璃前端已重新接入总线，LTPX 派发通道恢复")
			return
		}
		time.Sleep(frontendRevivePollInterval)
	}
	LoggerGeneral.Warn("CrystalAstral", "等待琉璃前端重新就绪超时（%v），本次 LTPX 派发可能无人接收", frontendReviveTimeout)
}
