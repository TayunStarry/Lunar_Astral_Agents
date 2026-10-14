//go:build tui

package app

import (
	"CorePersonality/transport/ws"
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/TuiSystem"
)

// WireTUIFrontendHooks 将前端在线探针与窗口唤起器注入 TUI 管理屏（仅 TUI 构建调用）：
//   - 探针以 WS 总线频道订阅数为判据（月华终端 / 琉璃桌面各自声明独立频道）；
//   - 唤起器复用装配层的窗口规格，重建桌面窗口（已存在时由 BrowserClient 聚焦）。
func WireTUIFrontendHooks() {
	TuiSystem.SetFrontendHooks(
		// 月华前端探针：消息终端渲染引擎频道的订阅数
		func() bool {
			return ws.Instance != nil && ws.Instance.ChannelBusCount(lunarRendererChannel) > 0
		},
		// 琉璃前端探针：引擎总线频道（DefaultBusChannel）的订阅数
		func() bool {
			return ws.Instance != nil && ws.Instance.ChannelBusCount(ws.DefaultBusChannel) > 0
		},
		// 月华窗口唤起器
		func() error {
			return BrowserClient.OpenDesktopWindow(lunarWindowSpec())
		},
		// 琉璃窗口唤起器
		func() error {
			return BrowserClient.OpenDesktopWindow(crystalWindowSpec())
		},
	)
}
