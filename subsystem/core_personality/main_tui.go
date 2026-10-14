//go:build tui

package main

import (
	"flag"
	"os"

	"CorePersonality/app"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/TuiSystem"
	"LunarSubsystem/TuiSystem/updatepack"
)

// main TUI 控制台构建入口（go build -tags tui）：
// 在一切子系统装配前接管 stdout 日志管线，随后走标准装配流程，
// 最后以全屏 TUI 承载日志视图、运行状态、前端管理与更新包功能。
func main() {
	flag.Parse()

	// 日志管线安装必须最早期：接管 stdout/stderr，启动期日志不逃逸到终端
	TuiSystem.Install()

	// 前端全关自动退出：TUI 构建默认停用（配置 tui.exit_when_frontends_closed 显式开启时生效）
	if GeneralConfig.ExitWhenFrontendsClosed == nil {
		disabled := false
		GeneralConfig.ExitWhenFrontendsClosed = &disabled
	}

	// 注入更新包配置（lunar_config.json update_pack 节；空字段由 updatepack 内部兜底）
	TuiSystem.SetUpdatePackConfig(&updatepack.Config{
		GiteeOwner: GeneralConfig.UpdatePackGiteeOwner,
		GiteeRepo:  GeneralConfig.UpdatePackGiteeRepo,
		GiteeToken: GeneralConfig.UpdatePackGiteeToken,
		Include:    GeneralConfig.UpdatePackInclude,
		TargetDir:  GeneralConfig.UpdatePackTargetDir,
	})

	// 装配各子系统（传输层 / 人格层 / 引擎层 / HTTP 服务；日志经管线进入 TUI 视图）
	app.Assemble()

	// 前端管理钩子：在线探针与窗口唤起器注入 TUI 管理屏
	app.WireTUIFrontendHooks()

	// 打开两个桌面前端窗口：月华消息终端（/）与琉璃桌面（/desktop）
	app.OpenDesktopWindows()

	// TUI 全屏控制台：阻塞直至用户退出（q 键或中断信号）
	if err := TuiSystem.Run(); err != nil {
		LoggerGeneral.Error("CorePersonality", "TUI 控制台异常退出: %v", err)
	}

	// TUI 退出后优雅关闭全部子系统
	app.GracefulShutdown("TUI 控制台已退出")

	// 确保进程彻底退出（Windows 上 WebView2 COM 可能残留引用）
	os.Exit(0)
}
