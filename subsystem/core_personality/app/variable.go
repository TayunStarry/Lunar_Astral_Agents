package app

import (
	"net/http"
	"sync"
	"time"

	"CorePersonality/server"
)

// shutdownTimeout 优雅关闭的兜底超时。
// 卡在 webview Run() 中的窗口可能使 Terminate / HTTP 关闭无法及时返回，
// 超时后不再等待，由 main 的 os.Exit(0) 强制结束进程。
const shutdownTimeout = 3 * time.Second

// ==== 前端会话监视参数 ====

const (
	// sessionSampleInterval 会话数采样间隔
	sessionSampleInterval = time.Second
	// sessionQuietSamples 判定「页面已全部关闭」所需的连续空会话采样次数
	// （避开刷新/导航时的瞬时断连）
	sessionQuietSamples = 3
)

// ==== 桌面窗口标识 ====
const (
	// WindowLunar 月华消息终端窗口
	WindowLunar = "lunar"
	// WindowCrystal 琉璃桌面窗口
	WindowCrystal = "crystal"
)

// ==== WebSocket 总线频道（与前端页面声明的 channel 参数对齐） ====
const (
	// lunarRendererChannel 月华消息终端渲染引擎频道
	lunarRendererChannel = "message-terminal-renderer"
)

// ==== 桌面窗口图标（相对可执行文件目录；exe 输出在仓库根目录） ====
const (
	lunarIconPath   = "subsystem/core_personality/icon_lunar.ico"
	crystalIconPath = "subsystem/core_personality/icon_crystal.ico"
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

// ==== 退出信号 ====

// shutdownCh 进程内退出信号（前端会话全部断开等应用级触发）
var shutdownCh = make(chan struct{})

// shutdownOnce 保证退出信号只发一次
var shutdownOnce sync.Once

// notifyShutdown 请求进程退出（幂等）
func notifyShutdown() {
	shutdownOnce.Do(func() { close(shutdownCh) })
}
