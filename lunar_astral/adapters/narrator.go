package adapters

import (
	"LunarSubsystem/general_logger"
	"LunarSubsystem/qwen3_tts_lunar/module"
	"encoding/base64"
	"fmt"

	"github.com/dop251/goja"
)

// tts 适配TypeScript调用的文本转语音功能
// 接收文本参数，调用TTS合成引擎生成音频，Base64编码后通过WebSocket广播至所有客户端
// 返回值: [string, Error | null] 包含合成结果的元组，[音频Base64编码, 错误信息]
func (class *Runtime) tts(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足：需要提供文本内容")})
	}

	text, ok := call.Argument(0).Export().(string)
	if !ok || text == "" {
		return class.runtime.ToValue([]any{"", fmt.Errorf("文本内容不能为空")})
	}

	// 解析可选参数
	req := module.TTSRequest{
		Text:              text,
		Temperature:       0.8,
		TopK:              50,
		TopP:              0.9,
		MaxTokens:         2048,
		RepetitionPenalty: 1.1,
		Threads:           4,
	}

	// 第二个可选参数：配置对象
	if len(call.Arguments) >= 2 {
		if params, ok := call.Argument(1).Export().(map[string]any); ok {
			if v, ok := params["refAudio"].(string); ok {
				req.RefAudio = v
			}
			if v, ok := params["temperature"].(float64); ok {
				req.Temperature = float32(v)
			}
			if v, ok := params["topK"].(float64); ok {
				req.TopK = int32(v)
			}
			if v, ok := params["topP"].(float64); ok {
				req.TopP = float32(v)
			}
			if v, ok := params["maxTokens"].(float64); ok {
				req.MaxTokens = int32(v)
			}
			if v, ok := params["repetitionPenalty"].(float64); ok {
				req.RepetitionPenalty = float32(v)
			}
			if v, ok := params["threads"].(float64); ok {
				req.Threads = int32(v)
			}
			if v, ok := params["disableCache"].(bool); ok {
				req.DisableCache = v
			}
		}
	}

	// 执行TTS合成
	audioData, err := doTTSSynthesize(req)
	if err != nil {
		logger.Error("LunarCore", "TTS合成失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	logger.SubInfo("LunarCore", "TTS", "合成完成: [%s] 长度=%d", text, len(audioData))
	return class.runtime.ToValue([]any{audioData, nil})
}

// doTTSSynthesize 执行TTS合成，返回base64编码的WAV音频数据
func doTTSSynthesize(req module.TTSRequest) (string, error) {
	samples, err := module.SynthesizeText(
		req.Text, req.RefAudio, req.LanguageID,
		req.Temperature, req.TopK, req.TopP,
		req.MaxTokens, req.RepetitionPenalty, req.Threads,
	)
	if err != nil {
		return "", err
	}

	wavData := module.EncodePCMToWAV(samples, 24000)
	audioBase64 := base64.StdEncoding.EncodeToString(wavData)
	return audioBase64, nil
}
