package broadcaster

import (
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	KokoroTTS "LunarSubsystem/LunarHarness/engine/broadcaster/kokoro"
	OmniVoiceTTS "LunarSubsystem/LunarHarness/engine/broadcaster/omnivoice"
	qwentts "LunarSubsystem/LunarHarness/engine/broadcaster/qwen"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 统一合成执行层 ====
//
// Synthesize 是播报链路的唯一合成入口（HTTP 端点与进程内调用共用）：
//   1. cacheLookup     先查统一合成缓存；命中直接返回，不触碰引擎槽位；
//   2. resolveEngine   解析本次调用的引擎（请求覆写 → tts.engine 配置）；
//   3. ensureSlot      槽位对齐——目标引擎未在位时执行「卸载在位 → 加载目标」热切换，
//      合成与切换共用 slotMu 串行化：在位引擎合成进行中，切换请求自然等待其完毕；
//   4. synth*          引擎适配执行，统一参数按引擎格式转换，不支持的参数自动忽略；
//   5. cacheStore      成功结果回写缓存，并向同一请求的等待者广播（单飞防击穿）。
//
// 全部成功响应统一携带 engine、elapsed_ms 与 cached 字段；引擎特有字段（voice/phonemes 等）
// 按引擎能力追加，完整参数支持范围见 capabilities 矩阵。

// Synthesize 按统一参数执行合成，返回响应载荷与 HTTP 状态码
func Synthesize(req TTSRequest) (map[string]any, int) {
	// 维护态：一律拒绝（缓存命中同样不响应），避免维护期间拉起 TTS 模型
	if Paused() {
		return errPayload(errMaintenanceMessage), http.StatusServiceUnavailable
	}

	engine := resolveEngine(req.Engine)
	cacheKey := buildSynthCacheKey(engine, req)

	// 缓存命中：直接返回，不加载/切换任何引擎
	if !req.DisableCache {
		if audio, hit := synthCache.Get(cacheKey); hit {
			return synthPayload(engine, audio, 0, true), http.StatusOK
		}
	}

	// 单飞：同一请求的并发调用共享一次合成，避免缓存击穿
	inflight, exists := synthCache.GetOrCreateInflight(cacheKey)
	if exists {
		logger.SubInfo(logModule, logModuleCache, "等待已有合成完成")
		audio, err := inflight.Wait()
		if err != nil {
			return errPayload(err.Error()), http.StatusInternalServerError
		}
		return synthPayload(engine, audio, 0, false), http.StatusOK
	}
	defer synthCache.RemoveInflight(cacheKey)

	payload, status, elapsed := synthesizeOnce(engine, req)
	audio, _ := payload["audio"].(string)
	if status == http.StatusOK && audio != "" {
		synthCache.Set(cacheKey, audio)
		inflight.Complete(audio, nil)
	} else {
		msg, _ := payload["error"].(string)
		if msg == "" {
			msg = "TTS 合成失败"
		}
		inflight.Complete("", errors.New(msg))
	}

	payload["engine"] = engine
	payload["elapsed_ms"] = elapsed
	payload["cached"] = false
	return payload, status
}

// synthesizeOnce 槽位对齐并执行一次实际合成（缓存未命中路径）
func synthesizeOnce(engine string, req TTSRequest) (map[string]any, int, int64) {
	slotMu.Lock()
	defer slotMu.Unlock()

	// 维护态可能在「入口检查 → 拿到槽位锁」之间置位：拿锁后二次确认，
	// 保证维护期间不会真正加载引擎或执行合成。
	if Paused() {
		return errPayload(errMaintenanceMessage), http.StatusServiceUnavailable, 0
	}

	// 槽位对齐：单载语义——本次调用选中的引擎必须是在位的唯一引擎
	if err := ensureSlot(engine); err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError, 0
	}

	setSlot(engine, int32(stateSynthesizing))
	start := time.Now()
	payload, status := synthesizeWithEngine(engine, req)
	setSlot(engine, int32(stateReady))
	slotLastUseUnix.Store(nowUnix())
	return payload, status, time.Since(start).Milliseconds()
}

// synthPayload 缓存 / 单飞命中时的统一成功载荷
func synthPayload(engine, audio string, elapsedMS int64, cached bool) map[string]any {
	return map[string]any{
		"success":    true,
		"audio":      audio,
		"engine":     engine,
		"elapsed_ms": elapsedMS,
		"cached":     cached,
	}
}

// SynthesizeBase64 进程内合成入口：返回 base64 WAV 音频（Go 对话链语音合成使用）
func SynthesizeBase64(req TTSRequest) (string, error) {
	payload, status := Synthesize(req)
	if status != http.StatusOK {
		msg, _ := payload["error"].(string)
		if msg == "" {
			msg = "TTS 合成失败"
		}
		return "", errors.New(msg)
	}
	audio, _ := payload["audio"].(string)
	return audio, nil
}

// synthesizeWithEngine 引擎分发（槽位已对齐、slotMu 持有中调用）
func synthesizeWithEngine(engine string, req TTSRequest) (map[string]any, int) {
	switch engine {
	case EngineKokoro:
		return synthKokoro(req)
	case EngineOmniVoice:
		return synthOmniVoice(req)
	default:
		return synthQwen(req)
	}
}

// errPayload 统一错误响应载荷
func errPayload(msg string) map[string]any {
	return map[string]any{"success": false, "error": msg}
}

// ==== 参数回退辅助 ====

// firstNonEmpty 返回第一个非空字符串（请求参数优先，配置默认值兜底）
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// ptrOr 返回指针指向的值；指针为 nil（请求未携带）时回退配置默认值。
// 用于区分「未携带」与「显式零值（0/false）」：请求显式给出即覆写配置。
func ptrOr[T any](req *T, def T) T {
	if req != nil {
		return *req
	}
	return def
}

// ==== 引擎适配器 ====

// synthQwen Qwen 适配：统一参数 → Qwen 请求（采样控制与声音克隆）。
// 合成缓存由统一入口层负责，引擎包只做一次实际合成。
func synthQwen(req TTSRequest) (map[string]any, int) {
	refAudio := resolveAssetPath(firstNonEmpty(req.RefAudio, config.TTSRefAudio))
	samples, err := qwentts.SynthesizeText(
		req.Text, refAudio, req.LanguageID,
		req.Temperature, req.TopK, req.TopP, req.MaxTokens, req.RepetitionPenalty, req.Threads,
	)
	if err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}
	wav := qwentts.EncodePCMToWAV(samples, 24000)
	return map[string]any{
		"success":     true,
		"audio":       base64.StdEncoding.EncodeToString(wav),
		"sample_rate": 24000,
	}, http.StatusOK
}

// synthKokoro Kokoro 适配：统一参数 → Kokoro 请求。
// Kokoro 不支持参考音频：RefAudio/RefText 被丢弃；音色由 Voice/Mix 决定。
func synthKokoro(req TTSRequest) (map[string]any, int) {
	voice := firstNonEmpty(req.Voice, config.TTSVoice)
	speed := ptrOr(req.Speed, float32(config.TTSSpeed))
	lang := firstNonEmpty(req.Lang, config.TTSLang)
	mix := req.Mix
	if len(mix) == 0 && len(config.TTSMix) > 0 {
		for _, m := range config.TTSMix {
			mix = append(mix, MixEntry{Voice: m.Voice, Weight: m.Weight})
		}
	}
	var kmix []KokoroTTS.MixVoice
	for _, m := range mix {
		kmix = append(kmix, KokoroTTS.MixVoice{Voice: m.Voice, Weight: m.Weight})
	}

	// WithEngine 读锁钉住引擎：合成期间引擎不会被卸载
	var samples []float32
	var phonemes string
	err := KokoroTTS.WithEngine(func(engine *KokoroTTS.Engine) error {
		var serr error
		samples, phonemes, serr = engine.Synthesize(req.Text, voice, speed, lang, kmix)
		return serr
	})
	if err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}

	wav := KokoroTTS.EncodePCMToWAV(samples, KokoroTTS.SampleRate)
	return map[string]any{
		"success":     true,
		"audio":       base64.StdEncoding.EncodeToString(wav),
		"voice":       voice,
		"phonemes":    phonemes,
		"sample_rate": KokoroTTS.SampleRate,
	}, http.StatusOK
}

// synthOmniVoice OmniVoice 适配：统一参数 → OmniVoice 请求。
// 支持参考音频+参考文本声音克隆，或 instruct 属性关键词设计音色（二者互斥）。
func synthOmniVoice(req TTSRequest) (map[string]any, int) {
	seed := ptrOr(req.Seed, config.TTSSeed)

	// WithEngine 读锁钉住引擎：合成期间引擎不会被卸载
	var samples []float32
	err := OmniVoiceTTS.WithEngine(func(engine *OmniVoiceTTS.Engine) error {
		var serr error
		samples, serr = engine.Synthesize(OmniVoiceTTS.TTSRequest{
			Text:            req.Text,
			Lang:            firstNonEmpty(req.Lang, config.TTSLang),
			Instruct:        firstNonEmpty(req.Instruct, config.TTSInstruct),
			RefAudio:        resolveAssetPath(firstNonEmpty(req.RefAudio, config.TTSRefAudio)),
			RefText:         firstNonEmpty(req.RefText, config.TTSRefText),
			Seed:            seed,
			DisablePostproc: ptrOr(req.DisablePostproc, config.TTSDisablePostproc),
		})
		return serr
	})
	if err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}

	wav := OmniVoiceTTS.EncodePCMToWAV(samples, OmniVoiceTTS.SampleRate)
	voice := firstNonEmpty(req.Instruct, config.TTSInstruct)
	if voice == "" {
		voice = firstNonEmpty(req.RefAudio, config.TTSRefAudio)
		if voice != "" {
			voice = "clone"
		}
	}
	return map[string]any{
		"success":     true,
		"audio":       base64.StdEncoding.EncodeToString(wav),
		"voice":       voice,
		"sample_rate": OmniVoiceTTS.SampleRate,
	}, http.StatusOK
}
