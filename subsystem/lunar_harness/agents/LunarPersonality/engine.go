package LunarPersonality

// engine.go 前端在线判定与语音合成桥接。
//
// 动作匹配已移交前端引擎：消息推送后由 lunar.live2d_engine / lunar.rendering_engine
// 各自调用嵌入模型匹配自己的动作列表，后端不再读取 animation_list / telemetry，
// 也不再下发 action / movement 指令。

import (
	broadcaster "LunarSubsystem/LunarHarness/engine/broadcaster"
	proxy "LunarSubsystem/LunarHarness/engine/console/proxy"
	"LunarSubsystem/LunarHarness/transport/ws"
)

// frontendOnline 月华前端或 HTTPS 代理是否在线
func frontendOnline() bool {
	if ws.Instance != nil && ws.Instance.ChannelClientCount(ws.ChannelLunarTerminal) > 0 {
		return true
	}
	return proxy.IsRunning()
}

// synthesizeText 经统一播报链路合成语音
func synthesizeText(text string) (string, error) {
	return broadcaster.SynthesizeBase64(broadcaster.TTSRequest{
		Text:              text,
		Temperature:       0.8,
		TopK:              50,
		TopP:              0.9,
		MaxTokens:         2048,
		RepetitionPenalty: 1.1,
		Threads:           4,
	})
}
