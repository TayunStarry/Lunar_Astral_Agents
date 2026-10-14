//go:build !tui

package main

import (
	"flag"
	"os"

	"CorePersonality/app"
)

func main() {
	flag.Parse()

	// 装配各子系统（传输层 / 协议层 / 人格层 / 引擎层 / HTTP 服务）
	app.Assemble()

	// 打开两个桌面前端窗口：月华消息终端（/）与琉璃桌面（/desktop）
	app.OpenDesktopWindows()

	// 阻塞至退出信号或全部桌面窗口关闭，优雅关闭
	app.WaitForShutdown()

	// 确保进程彻底退出（Windows 上 WebView2 COM 可能残留引用）
	os.Exit(0)
}
