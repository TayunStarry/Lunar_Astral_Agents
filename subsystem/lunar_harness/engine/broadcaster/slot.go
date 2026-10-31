package broadcaster

import (
	"fmt"
	"time"

	KokoroTTS "LunarSubsystem/LunarHarness/engine/broadcaster/kokoro"
	OmniVoiceTTS "LunarSubsystem/LunarHarness/engine/broadcaster/omnivoice"
	qwentts "LunarSubsystem/LunarHarness/engine/broadcaster/qwen"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 引擎槽位：单载生命周期（Broadcaster 核心） ====
//
// 设计语言 · 槽位语义：
//   - 单载：任一时刻至多一个 TTS 引擎处于加载状态——即「被本次调用选中的引擎」；
//     目标引擎与在位引擎不同时，先卸载在位引擎再加载目标引擎；
//   - 切换序列：等待在位合成完毕（slotMu 串行化天然保证）→ 卸载在位引擎 →
//     加载目标引擎 → 执行本次调用；
//   - 观测：槽位状态经原子变量暴露给 /broadcaster/status，观测零锁且无副作用；
//   - 回收：闲置卸载守护（lifecycle.go）周期检查槽位活动时间戳，超阈值卸载在位
//     引擎，下一次调用自动重新加载。

// resolveEngine 解析本次调用使用的引擎：请求 engine 覆写值优先，
// 空或非法取值回退 tts.engine 配置（general_config 校验保证合法，default 兜底 Qwen）。
func resolveEngine(override string) string {
	switch override {
	case EngineQwen, EngineKokoro, EngineOmniVoice:
		return override
	}
	return config.TTSEngine
}

// currentEngine 读取在位引擎名（"" = 空槽）
func currentEngine() string {
	if name, ok := slotEngineName.Load().(string); ok {
		return name
	}
	return ""
}

// setSlot 在 slotMu 保护内写入槽位可见状态（引擎名 + 状态机）
func setSlot(engine string, state int32) {
	slotEngineName.Store(engine)
	slotStateVal.Store(state)
}

// Activate 显式激活引擎：把槽位切换到指定引擎并等待加载完成（预加载，消除首响延迟）。
// 已在位时幂等；其它引擎在位时执行卸载→加载切换。进程内调用入口（activate 端点、
// 引擎专属能力端点共用）。
func Activate(engine string) error {
	slotMu.Lock()
	defer slotMu.Unlock()
	return ensureSlot(engine)
}

// Release 显式卸载在位引擎（幂等：空槽为空操作）
func Release() error {
	slotMu.Lock()
	defer slotMu.Unlock()
	return unloadSlot()
}

// ensureSlot 槽位对齐：目标引擎已在位直接返回；否则卸载在位引擎并加载目标引擎。
// 须持有 slotMu。
func ensureSlot(engine string) error {
	// 维护态：不加载任何引擎（合成入口与专属能力端点共用此守卫）
	if Paused() {
		return fmt.Errorf("%s", errMaintenanceMessage)
	}
	if currentEngine() == engine {
		return nil
	}
	// 卸载在位引擎（合成互斥由 slotMu 保证；引擎内部 Unload 亦会等待自身合成收尾）
	if old := currentEngine(); old != "" {
		setSlot(old, int32(stateUnloading))
		if err := unloadEngine(old); err != nil {
			setSlot("", int32(stateUnloaded))
			return fmt.Errorf("卸载 %s 引擎失败: %w", old, err)
		}
	}
	// 加载目标引擎
	setSlot("", int32(stateLoading))
	if err := loadEngine(engine); err != nil {
		setSlot("", int32(stateUnloaded))
		return err
	}
	setSlot(engine, int32(stateReady))
	slotLastUseUnix.Store(nowUnix())
	n := switchCount.Add(1)
	logger.Info(logModule, "%s 引擎已加载（槽位切换累计 %d 次）", engine, n)
	return nil
}

// unloadSlot 卸载在位引擎并清空槽位（幂等）。须持有 slotMu。
func unloadSlot() error {
	engine := currentEngine()
	if engine == "" {
		return nil
	}
	setSlot(engine, int32(stateUnloading))
	if err := unloadEngine(engine); err != nil {
		setSlot(engine, int32(stateReady))
		return fmt.Errorf("卸载 %s 引擎失败: %w", engine, err)
	}
	setSlot("", int32(stateUnloaded))
	logger.Info(logModule, "%s 引擎已卸载（槽位空闲，下次调用时重新加载）", engine)
	return nil
}

// Snapshot 槽位观测快照（零锁：仅读原子变量，不触碰引擎与槽位锁）
func Snapshot() SlotSnapshot {
	snap := SlotSnapshot{
		Engine:        currentEngine(),
		State:         SlotState(stateNames[slotStateVal.Load()]),
		DefaultEngine: config.TTSEngine,
		LastUsedUnix:  slotLastUseUnix.Load(),
		SwitchCount:   switchCount.Load(),
	}
	if snap.Engine == EngineOmniVoice {
		if eng := OmniVoiceTTS.GetEngine(); eng != nil {
			snap.Version = eng.Version()
		}
	}
	return snap
}

// stateNames 槽位状态枚举 → 设计语言状态名
var stateNames = map[int32]SlotState{
	stateUnloaded:     SlotUnloaded,
	stateLoading:      SlotLoading,
	stateReady:        SlotReady,
	stateSynthesizing: SlotSynthesizing,
	stateUnloading:    SlotUnloading,
}

// ==== 引擎加载 / 卸载分发 ====

// loadEngine 加载指定引擎（ensure.go 的加载器分发）
func loadEngine(name string) error {
	switch name {
	case EngineKokoro:
		return EnsureKokoro()
	case EngineOmniVoice:
		return EnsureOmniVoice()
	default:
		return EnsureQwen()
	}
}

// unloadEngine 卸载指定引擎（各引擎 Unload 幂等，且等待引擎内进行中的合成完成）
func unloadEngine(name string) error {
	switch name {
	case EngineKokoro:
		return KokoroTTS.UnloadEngine()
	case EngineOmniVoice:
		return OmniVoiceTTS.UnloadEngine()
	default:
		return qwentts.UnloadTTSEngine()
	}
}

// nowUnix 当前 unix 秒
func nowUnix() int64 { return time.Now().Unix() }
