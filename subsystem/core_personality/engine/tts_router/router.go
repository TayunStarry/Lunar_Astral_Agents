// Package ttsrouter 月华语音合成统一路由：把统一参数样式的合成请求分发到
// Qwen / Kokoro / OmniVoice 引擎执行。
//
// 职责边界：
//   - 参数层面统一：TTSRequest 为唯一请求形态；配置文件「tts」节仅作默认值来源，
//     请求携带的参数（含 engine 覆写）一律优先于配置；
//   - 执行层面适配：各 synth* 按引擎自身格式构造请求，引擎不支持的参数自动忽略
//     （如 Kokoro 不支持参考音频）；
//   - 生命周期托管：引擎惰性加载（ensure.go）+ 5 分钟闲置自动卸载（lifecycle.go）。
//
// 所有月华侧 TTS 调用路径（/tts 端点、JS 运行时 textToSpeech 绑定、LTP9 包工具）
// 均收敛到本包；切换引擎默认改配置，也可按请求携带 engine 参数临时覆写。
package ttsrouter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	KokoroTTS "CorePersonality/engine/kokoro_tts"
	OmniVoiceTTS "CorePersonality/engine/omnivoice_tts"
	qwentts "CorePersonality/engine/qwen_tts"
	"LunarSubsystem/GeneralConfig"
)

// TTSDispatchHandler 月华 /tts 统一入口：按请求 engine 覆写值分发到所选引擎，
// 未携带或取值非法时回退 tts.engine 配置。需要随配置切换引擎的调用方使用本端点；
// 明确锁定引擎的场景请用 /qwen/tts、/kokoro/tts、/omnivoice/tts 专用端点。
func TTSDispatchHandler(w http.ResponseWriter, r *http.Request) {
	req, ok := serveTTSPOST(w, r)
	if !ok {
		return
	}
	payload, status := Synthesize(req)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

// serveTTSPOST 统一端点与专用端点共享的 HTTP 前置处理：
// CORS/方法守卫 + 统一参数解析 + text 非空校验。
// 通过校验时返回解析后的请求；失败时已写好响应并返回 false。
func serveTTSPOST(w http.ResponseWriter, r *http.Request) (TTSRequest, bool) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return TTSRequest{}, false
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return TTSRequest{}, false
	}

	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "无效的请求格式"})
		return TTSRequest{}, false
	}
	if strings.TrimSpace(req.Text) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "文本内容不能为空"})
		return TTSRequest{}, false
	}
	return req, true
}

// Synthesize 按引擎执行统一参数合成，返回响应载荷与 HTTP 状态码（进程内调用复用）。
// 引擎优先取请求携带的 engine 覆写值；空或非法取值回退 tts.engine 配置。
func Synthesize(req TTSRequest) (map[string]any, int) {
	engine := *GeneralConfig.TTSEngine
	switch req.Engine {
	case "Qwen", "Kokoro", "OmniVoice":
		engine = req.Engine
	}
	// 配置校验（init.go）保证枚举合法，default 兜底按 Qwen 处理
	switch engine {
	case "Kokoro":
		return synthKokoro(req)
	case "OmniVoice":
		return synthOmniVoice(req)
	default:
		return synthQwen(req)
	}
}

// SynthesizeBase64 进程内合成入口：返回 base64 WAV 音频（月华 JS 运行时 textToSpeech 绑定使用）
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

// ==== 引擎适配执行层 ====

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

// synthQwen Qwen 适配：统一参数 → Qwen 请求（保留采样控制与合成缓存）。
// Qwen 支持参考音频声音克隆；voice/speed/mix 等音色参数被忽略。
func synthQwen(req TTSRequest) (map[string]any, int) {
	if err := EnsureQwen(); err != nil {
		return errPayload("Qwen 引擎加载失败: " + err.Error()), http.StatusInternalServerError
	}

	qreq := qwentts.TTSRequest{
		Text:              req.Text,
		RefAudio:          resolveAssetPath(firstNonEmpty(req.RefAudio, *GeneralConfig.TTSRefAudio)),
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
	if err := EnsureKokoro(); err != nil {
		return errPayload("Kokoro 引擎加载失败: " + err.Error()), http.StatusInternalServerError
	}

	voice := firstNonEmpty(req.Voice, *GeneralConfig.TTSVoice)
	speed := ptrOr(req.Speed, float32(*GeneralConfig.TTSSpeed))
	lang := firstNonEmpty(req.Lang, *GeneralConfig.TTSLang)
	mix := req.Mix
	if len(mix) == 0 && len(GeneralConfig.TTSMix) > 0 {
		for _, m := range GeneralConfig.TTSMix {
			mix = append(mix, KokoroTTS.MixVoice{Voice: m.Voice, Weight: m.Weight})
		}
	}

	// WithEngine 读锁钉住引擎：合成期间不会被闲置卸载守护销毁
	var samples []float32
	var phonemes string
	err := KokoroTTS.WithEngine(func(engine *KokoroTTS.Engine) error {
		var serr error
		samples, phonemes, serr = engine.Synthesize(req.Text, voice, speed, lang, mix)
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
	if err := EnsureOmniVoice(); err != nil {
		return errPayload("OmniVoice 引擎加载失败: " + err.Error()), http.StatusInternalServerError
	}

	seed := ptrOr(req.Seed, *GeneralConfig.TTSSeed)

	// WithEngine 读锁钉住引擎：合成期间不会被闲置卸载守护销毁
	var samples []float32
	err := OmniVoiceTTS.WithEngine(func(engine *OmniVoiceTTS.Engine) error {
		var serr error
		samples, serr = engine.Synthesize(OmniVoiceTTS.TTSRequest{
			Text:            req.Text,
			Lang:            firstNonEmpty(req.Lang, *GeneralConfig.TTSLang),
			Instruct:        firstNonEmpty(req.Instruct, *GeneralConfig.TTSInstruct),
			RefAudio:        resolveAssetPath(firstNonEmpty(req.RefAudio, *GeneralConfig.TTSRefAudio)),
			RefText:         firstNonEmpty(req.RefText, *GeneralConfig.TTSRefText),
			Seed:            seed,
			DisablePostproc: ptrOr(req.DisablePostproc, *GeneralConfig.TTSDisablePostproc),
		})
		return serr
	})
	if err != nil {
		return errPayload(err.Error()), http.StatusInternalServerError
	}

	wav := OmniVoiceTTS.EncodePCMToWAV(samples, OmniVoiceTTS.SampleRate)
	voice := firstNonEmpty(req.Instruct, *GeneralConfig.TTSInstruct)
	if voice == "" {
		voice = firstNonEmpty(req.RefAudio, *GeneralConfig.TTSRefAudio)
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

// errPayload 统一错误响应载荷
func errPayload(msg string) map[string]any {
	return map[string]any{"success": false, "error": msg}
}
