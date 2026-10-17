package lunarloop

import (
	logger "LunarSubsystem/LoggerGeneral"
	"fmt"

	ttsrouter "CorePersonality/engine/tts_router"
	"CorePersonality/transport/ws"
	proxy "LunarSubsystem/TuiSystem/proxy"

	"github.com/dop251/goja"
)

// lunarFrontendOnline 月华前端在线探测：消息终端频道（桌面端/手机端共用）
// 存在任一 WebSocket 连接即视为在线。
func lunarFrontendOnline() bool {
	return ws.Instance != nil && ws.Instance.ChannelClientCount(ws.ChannelLunarTerminal) > 0
}

// textToSpeech 适配TypeScript调用的文本转语音功能
// 接收文本参数，经统一 TTS 路由（按 tts.engine 配置选择引擎）合成音频，
// Base64编码后通过WebSocket广播至所有客户端
// 返回值: [string, Error | null] 包含合成结果的元组，[音频Base64编码, 错误信息]
func (class *Runtime) textToSpeech(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{"", fmt.Errorf("参数不足：需要提供文本内容")})
	}

	text, ok := call.Argument(0).Export().(string)
	if !ok || text == "" {
		return class.runtime.ToValue([]any{"", fmt.Errorf("文本内容不能为空")})
	}

	// 音频消费方探测：本地前端经 /ws 直连、远程前端经 HTTPS 代理接入，
	// 两者皆不在线时音频无人消费，跳过合成流程仅推送文本
	// （调用方对合成失败已有降级：语音为空时照常推送文本内容）
	if !lunarFrontendOnline() && !proxy.IsRunning() {
		logger.SubInfo("LunarCore", "TTS", "前端不在线且HTTPS代理未启用，跳过合成（仅推送文本）")
		return class.runtime.ToValue([]any{"", fmt.Errorf("月华前端不在线且HTTPS代理未启用，跳过TTS合成")})
	}

	// 解析可选参数（采样控制仅 Qwen 引擎生效，其余引擎忽略）
	req := ttsrouter.TTSRequest{
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
			// 数值一律走 asFloat/asInt：goja 把 JS 整数导出为 int64、小数导出为 float64，
			// 只断言 float64 会让整数型参数（topK / maxTokens / threads）静默沿用默认值
			if v := asFloat(params["temperature"]); v > 0 {
				req.Temperature = float32(v)
			}
			if v := asInt(params["topK"]); v > 0 {
				req.TopK = int32(v)
			}
			if v := asFloat(params["topP"]); v > 0 {
				req.TopP = float32(v)
			}
			if v := asInt(params["maxTokens"]); v > 0 {
				req.MaxTokens = int32(v)
			}
			if v := asFloat(params["repetitionPenalty"]); v > 0 {
				req.RepetitionPenalty = float32(v)
			}
			if v := asInt(params["threads"]); v > 0 {
				req.Threads = int32(v)
			}
			if v, ok := params["disableCache"].(bool); ok {
				req.DisableCache = v
			}
		}
	}

	// 执行TTS合成（统一路由按配置引擎分发）
	audioData, err := ttsrouter.SynthesizeBase64(req)
	if err != nil {
		logger.Error("LunarCore", "TTS合成失败: %v", err)
		return class.runtime.ToValue([]any{"", err})
	}

	logger.SubInfo("LunarCore", "TTS", "合成完成: [%s] 长度=%d", text, len(audioData))
	return class.runtime.ToValue([]any{audioData, nil})
}
