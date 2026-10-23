package app

import (
	"net/http"
	"time"

	"LunarSubsystem/LunarHarness/server"
	"LunarSubsystem/LunarHarness/transport/ws"
)

// shutdownTimeout 优雅关闭的兜底超时。
// 卡在 webview Run() 中的窗口可能使 Terminate / HTTP 关闭无法及时返回，
// 超时后不再等待，由 main 的 os.Exit(0) 强制结束进程。
const shutdownTimeout = 3 * time.Second

// ==== 桌面前端窗口标识 ====
const (
	// WindowLunar 月华消息终端窗口
	WindowLunar = "lunar"
	// WindowCrystal 琉璃桌面窗口
	WindowCrystal = "crystal"
)

// ==== WebSocket 总线频道（与前端页面声明的 channel 参数对齐） ====
const (
	// lunarTerminalChannel 月华消息终端（含手机端）声明的接入频道。
	// 该终端不订阅引擎总线（bus=0），故在线判定按「频道连接数」而非「总线订阅数」统计。
	lunarTerminalChannel = ws.ChannelLunarTerminal
)

// ==== 桌面窗口图标（相对可执行文件目录；exe 输出在仓库根目录） ====
const (
	lunarIconPath   = "subsystem/lunar_harness/icon_lunar.ico"
	crystalIconPath = "subsystem/lunar_harness/icon_crystal.ico"
)

// ==== 窗口尺寸 ====
const (
	lunarWindowWidth    = 1080
	lunarWindowHeight   = 810
	crystalWindowWidth  = 1500
	crystalWindowHeight = 1050
)

// ==== 装配期共享状态 ====

// httpServer 当前运行的 HTTP 服务实例
var httpServer *http.Server

// StaticRootsProvider 两个桌面前端静态资源提供者（由人格层在装配时注入）
var StaticRootsProvider func() server.StaticRoots
