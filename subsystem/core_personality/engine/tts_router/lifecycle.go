package ttsrouter

import (
	"time"

	KokoroTTS "CorePersonality/engine/kokoro_tts"
	OmniVoiceTTS "CorePersonality/engine/omnivoice_tts"
	qwentts "CorePersonality/engine/qwen_tts"
	"LunarSubsystem/GeneralConfig"
	logger "LunarSubsystem/LoggerGeneral"
)

// ==== 闲置卸载守护 ====
//
// 三个 TTS 引擎均支持「惰性加载 + 闲置卸载」：加载在首次合成请求时发生
// （ensure.go），本守护周期检查各引擎最近一次合成调用的时间戳，超过阈值
// （默认 5 分钟，可经 tts.idle_unload_minutes 配置）即卸载模型占用。卸载后
// 下一次合成请求会自动重新加载。
//
// 「最近调用」以真实合成为准（touchEngine 只在引擎 Synthesize 路径更新），
// health/voices 等辅助端点不会阻止卸载。

// idleEngine 闲置卸载守护管理的引擎槽位
type idleEngine struct {
	// name 引擎显示名（日志用）
	name string
	// lastUsed 最近一次合成调用的 unix 秒（0 = 从未加载过）
	lastUsed func() int64
	// unload 卸载函数（幂等，内部等待进行中的合成完成后才销毁）
	unload func() error
}

// idleEngines 守护覆盖的三个引擎
func idleEngines() []idleEngine {
	return []idleEngine{
		{"Qwen", qwentts.LastUsedUnix, qwentts.UnloadTTSEngine},
		{"Kokoro", KokoroTTS.LastUsedUnix, KokoroTTS.UnloadEngine},
		{"OmniVoice", OmniVoiceTTS.LastUsedUnix, OmniVoiceTTS.UnloadEngine},
	}
}

// StartIdleUnloader 启动 TTS 闲置卸载守护（进程级单次启动，随 Assemble 调用）
func StartIdleUnloader() {
	// 阈值：启动时读一次配置；非法值（<=0）回退默认 5 分钟
	if *GeneralConfig.TTSIdleUnloadMinutes > 0 {
		idleUnloadSeconds = int64(*GeneralConfig.TTSIdleUnloadMinutes) * 60
	}
	go func() {
		ticker := time.NewTicker(ttsIdleScanInterval * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanIdleEngines()
		}
	}()
	logger.Info("TTS-Router", "闲置卸载守护已启动（%d 分钟无合成调用即卸载引擎）", idleUnloadSeconds/60)
}

// scanIdleEngines 单轮闲置扫描：超阈值即卸载
func scanIdleEngines() {
	now := time.Now().Unix()
	for _, slot := range idleEngines() {
		last := slot.lastUsed()
		// 0 = 从未加载；未超阈值跳过
		if last == 0 || now-last < idleUnloadSeconds {
			continue
		}
		// 已卸载且此后无新合成（lastUsed 未变）：跳过，避免每轮重复卸载与日志刷屏
		if idleUnloadState[slot.name] == last {
			continue
		}
		if err := slot.unload(); err != nil {
			logger.Error("TTS-Router", "%s 引擎闲置卸载失败: %v", slot.name, err)
			continue
		}
		// 记录本次卸载对应的 lastUsed：直到该引擎被重新加载并合成（时间戳更新）前不再重复上报
		idleUnloadState[slot.name] = last
		logger.Info("TTS-Router", "%s 引擎已闲置 %d 秒，卸载释放（下次调用时重新加载）", slot.name, now-last)
	}
}
