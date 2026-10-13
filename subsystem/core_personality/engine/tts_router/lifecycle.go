package ttsrouter

import (
	"time"

	KokoroTTS "CorePersonality/engine/kokoro_tts"
	OmniVoiceTTS "CorePersonality/engine/omnivoice_tts"
	qwentts "CorePersonality/engine/qwen_tts"
	logger "LunarSubsystem/LoggerGeneral"
)

// ==== 闲置卸载守护 ====
//
// 三个 TTS 引擎均支持「惰性加载 + 闲置卸载」：加载在首次合成请求时发生
// （ensure.go），本守护周期检查各引擎最近一次合成调用的时间戳，超过阈值
// （5 分钟）即卸载模型占用。卸载后下一次合成请求会自动重新加载。
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
	go func() {
		ticker := time.NewTicker(ttsIdleScanInterval * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanIdleEngines()
		}
	}()
	logger.Info("TTS-Router", "闲置卸载守护已启动（%d 秒无合成调用即卸载引擎）", ttsIdleUnloadSeconds)
}

// scanIdleEngines 单轮闲置扫描：超阈值即卸载
func scanIdleEngines() {
	now := time.Now().Unix()
	for _, slot := range idleEngines() {
		last := slot.lastUsed()
		// 0 = 从未加载；未超阈值跳过
		if last == 0 || now-last < ttsIdleUnloadSeconds {
			continue
		}
		if err := slot.unload(); err != nil {
			logger.Error("TTS-Router", "%s 引擎闲置卸载失败: %v", slot.name, err)
		} else {
			logger.Info("TTS-Router", "%s 引擎已闲置 %d 秒，卸载释放（下次调用时重新加载）", slot.name, now-last)
		}
	}
}
