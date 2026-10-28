package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"LunarSubsystem/LunarHarness/app"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/console/ltpxmarket"
	"LunarSubsystem/LunarHarness/engine/console/updatepack"
	"LunarSubsystem/LunarHarness/engine/sound"
	"LunarSubsystem/LunarHarness/server"
)

// main 程序唯一入口（Windows 上只有控制台形态）：
// 先做单实例闸门，再接管 stdout 日志管线，随后走标准装配流程，
// 最后以全屏 TUI 控制台承载日志视图、运行状态、智能体实况与运维功能，
// 并由控制台代为打开月华/琉璃两个前端窗口（开发者模式下不自动弹出，窗口全关不退出控制台）。
func main() {
	flag.Parse()

	// 单实例闸门：必须在装配任何子系统之前。
	// Windows 允许同一端口被多个套接字绑定（Go 不再设置 SO_EXCLUSIVEADDRUSE），
	// 重复启动时两个实例会同时「监听成功」——后启动者接管 HTTP 端口、两边各自拉起模型服务
	// 并互抢端口，表现为「前端 404 → 前后端一起崩溃退出」，且因为没有可见提示而被当成崩溃。
	ln, err := server.ListenInstance(config.BasicPort, config.AllowLAN)
	if err != nil {
		exitVisible(err.Error() + "\n  已有一个「星月智能」在运行：请勿重复启动。\n" +
			"  如需并存多个实例，请用 -basic-port 指定不同端口。")
	}
	// 预绑定监听器交由 HTTP 服务复用（避免预检与正式绑定之间的竞态窗口）
	server.SetPreboundListener(ln)

	// 控制台日志镜像默认关闭：日志仍进 TUI 的「原始日志」视图与「数据导出」，
	// 只有 lunar_config.json 的 system.console_log=true 时才额外落盘 local_data/logs/console.log。
	console.SetConsoleLogEnabled(config.ConsoleLogEnabled)

	// 日志管线安装必须最早期：接管 stdout/stderr，启动期日志不逃逸到终端
	console.Install()

	// 构建版本号注入控制台：运行状态屏显示本次编译产物的版本
	// （取自编译时生成并嵌入的端点自述文档 info.version）
	console.SetBuildVersion(server.DocsVersion())

	// 注入 Gitee Release 发布通道配置（lunar_config.json release 节）。
	// 更新包与 LTPX 应用市场共用同一仓库：仓库信息只有一套，
	// 两条通道在仓库内以「描述标记 + 附件名前缀」区分（见 TuiSystem/channel）。
	console.SetUpdatePackConfig(&updatepack.Config{
		GiteeOwner:      config.ReleaseGiteeOwner,
		GiteeRepo:       config.ReleaseGiteeRepo,
		GiteeToken:      config.ReleaseGiteeToken,
		Include:         config.ReleaseInclude,
		Exclude:         config.ReleaseExclude,
		ExcludeDirsWith: config.ReleaseExcludeDirsWith,
		BackupKeep:      config.ReleaseBackupKeep,
		ProgramFiles:    config.ReleaseProgramFiles,
		RestartArgs:     config.ReleaseRestartArgs,
		TargetDir:       config.ReleaseTargetDir,
	})

	// 令牌通常留空：发布时在 TUI 手动输入，不落盘
	console.SetMarketConfig(&ltpxmarket.Config{
		GiteeOwner: config.ReleaseGiteeOwner,
		GiteeRepo:  config.ReleaseGiteeRepo,
		PackageDir: config.ReleasePackageDir,
	})

	// 端口基准注入 TUI：代理屏的提示值必须来自真实配置，并拦住会与自身服务撞车的端口。
	// 实测教训：代理屏曾把「代理端口」提示写死为 36790——那正是模型服务端口（BasicPort+1）。
	// 用户照提示填写后，HTTPS 代理（Windows 允许多端口绑定）接管了模型服务端口，程序自身
	// 发往模型的请求打到 TLS 代理上，报 TLS 握手错误/连接重置/嵌入请求 EOF，连锁导致记忆库失败。
	console.SetProxyDefaults(config.BasicPort, config.ProxyPort, map[int]string{
		config.BasicPort: "Web 服务（前端与 API）",
		config.ModelPort: "模型服务（llama-server）",
	})

	// 装配各子系统（传输层 / 人格层 / 引擎层 / HTTP 服务；日志经管线进入 TUI 视图）
	app.Assemble()

	// 前端管理钩子：在线探针与窗口唤起器注入 TUI 管理屏
	app.WireTUIFrontendHooks()

	// 数据导出钩子：控制台「数据导出」屏据此把原始日志 / 智能体上下文落盘
	app.WireTUIDataDumpHooks()

	// LTP9 授权钩子：控制台「LTP9 授权签发」屏据此扫描包目录、读取授权现状并签发 permissions.key
	app.WireTUILTP9AuthHooks()

	// 对话链状态钩子：控制台「月华对话链 管理」屏据此展示 Go 对话链状态并手动重建
	app.WireTUIEngineHooks()

	// 打开两个前端窗口：月华消息终端（/）与琉璃桌面（/desktop）。
	// 开发者模式（system.developer 或 --developer）下**不主动弹出前端**：
	// 调试时只看控制台即可，避免窗口抢占焦点、遮挡日志；
	// 需要看页面时仍可在「系统管理 → 前端页面」里手动唤起。
	if config.Developer {
		logger.Info("LunarHarness", "开发者模式：跳过自动打开月华/琉璃前端窗口（可在控制台「前端页面」手动唤起）")
	} else {
		app.OpenFrontendWindows()
	}

	// TUI 全屏控制台：阻塞直至用户退出（z / Ctrl+C / 中断信号）
	if err := console.Run(); err != nil {
		logger.Error("LunarHarness", "TUI 控制台异常退出: %v", err)
	}

	// 控制台退出后优雅关闭全部子系统
	app.GracefulShutdown("TUI 控制台已退出")

	// 确保进程彻底退出（Windows 上 WebView2 COM 可能残留引用）
	os.Exit(0)
}

// exitVisible 打印启动期致命错误并退出。
//
// 两条硬要求：
//  1. 必须在 console.Install() **之前**调用——此后 stdout/stderr 会被日志管线接管，
//     写出去的内容只会进 TUI 的环形缓冲，在控制台界面上永远看不到（这正是"窗口一闪
//     就没了、完全不知道发生了什么"的成因）；
//  2. 双击启动时控制台会随进程退出而关闭，因此要留出阅读时间，否则再清楚的原因也读不到。
func exitVisible(msg string) {
	fmt.Fprintf(os.Stderr, "\n[星月智能] 启动失败：%s\n\n", msg)
	fmt.Fprintln(os.Stderr, "（本窗口将在 15 秒后自动关闭，按回车立即退出）")
	// 端口被占用等服务级启动失败：日志管线尚未安装，直接同步播放失败音效
	sound.FailureBlocking()
	done := make(chan struct{})
	go func() {
		b := make([]byte, 1)
		_, _ = os.Stdin.Read(b)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
	}
	os.Exit(1)
}
