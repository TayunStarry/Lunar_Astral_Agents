package broadcaster

import (
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	KokoroTTS "LunarSubsystem/LunarHarness/engine/kokoro_tts"
	OmniVoiceTTS "LunarSubsystem/LunarHarness/engine/omnivoice_tts"
	qwentts "LunarSubsystem/LunarHarness/engine/qwen_tts"
)

// ==== 统一合成执行层 ====
//
// Synthesize 是播报链路的唯一合成入口（HTTP 端点与进程内调用共用）：
//   1. resolveEngine  解析本次调用的引擎（请求覆写 → tts.engine 配置）；
//   2. ensureSlot     槽位对齐——目标引擎未在位时执行「卸载在位 → 加载目标」热切换，
//      合成与切换共用 slotMu 串行化：在位引擎合成进行中，切换请求自然等待其完毕；
//   3. synth*         引擎适配执行，统一参数按引擎格式转换，不支持的参数自动忽略。
//
// 全部成功响应统一携带 engine 与 elapsed_ms 字段；引擎特有字段（voice/phonemes 等）
// 按引擎能力追加，完整参数支持范围见 capabilities 矩阵。

// Synthesize 按统一参数执行合成，返回响应载荷与 HTTP 状态码
func Synthesize(req TTSRequest) (map[string]any, int) {
	engine := resolveEngine(req.Engine)

	slotMu.Lock()
	defer slotMu.Unlock()

	// 槽位对齐：单载语义——本次调用选中的引擎必须是在位的唯一引擎
	if err := ensureSlot(engine); err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}

	setSlot(engine, int32(stateSynthesizing))
	start := time.Now()
	payload, status := synthesizeWithEngine(engine, req)
	setSlot(engine, int32(stateReady))
	slotLastUseUnix.Store(nowUnix())

	if status == http.StatusOK {
		payload["engine"] = engine
		payload["elapsed_ms"] = time.Since(start).Milliseconds()
	}
	return payload, status
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

// synthQwen Qwen 适配：统一参数 → Qwen 请求（保留采样控制与合成缓存）。
// Qwen 支持参考音频声音克隆；voice/speed/mix 等音色参数被忽略。
func synthQwen(req TTSRequest) (map[string]any, int) {
	qreq := qwentts.TTSRequest{
		Text:              req.Text,
		RefAudio:          resolveAssetPath(firstNonEmpty(req.RefAudio, config.TTSRefAudio)),
		LanguageID:        req.LanguageID,
		Temperature:       req.Temperature,
		TopK:              req.TopK,
		TopP:              req.TopP,
		MaxTokens:         req.MaxTokens,
		RepetitionPenalty: req.RepetitionPenalty,
		Threads:           req.Threads,
		DisableCache:      req.DisableCache,
	}
	audio, err := qwentts.SynthesizeWithCache(&qreq)
	if err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}
	return map[string]any{"success": true, "audio": audio}, http.StatusOK
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
