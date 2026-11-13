package console

// tui_run.go 控制台 TUI 的启动、终端自检、后台转发与退出清理。

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/archive"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	"LunarSubsystem/LunarHarness/engine/console/portrelease"
	"LunarSubsystem/LunarHarness/engine/console/proxy"
)

// quitCleanup TUI 退出后的统一清理
func (m *appModel) quitCleanup() {
	if m.streamUnsub != nil {
		m.streamUnsub()
	}
	archive.SetLogHook(nil)
	archive.SetProgressHook(nil)
	proxy.SetLogHook(nil)
	portrelease.SetLogHook(nil)
}

// ErrNoTerminal 控制台输出未连接到终端（无法渲染界面）
var ErrNoTerminal = fmt.Errorf("输出未连接到终端：控制台界面无法显示")

// ensureTerminal 校验控制台可用，避免出现「隐形实例」。
//
// 这里只处理**最坏情况**：既没有可用的继承句柄，也取不到 CONOUT$（例如以服务/计划任务
// 方式在无控制台环境启动）。此时宁可明确报错退出，也不要留下一个看不见、按不动的后台
// 进程——它会占着端口挡住下一次启动。诊断直接写 CONOUT$，不依赖已被接管的标准流。
func ensureTerminal() error {
	if f, ok := GetRealStdout().(*os.File); ok && isConsoleFile(f) {
		return nil
	}
	msg := "[星月智能] 控制台界面无法显示：标准输出未连接到终端，且无法从控制台设备取得句柄。\r\n" +
		"  常见原因：以服务/计划任务等无控制台方式启动，或输出被重定向。\r\n" +
		"  本进程即将退出，以免留下看不见、按不动的后台实例占用端口。\r\n" +
		"  请在终端窗口中直接运行主程序；已打开的前端窗口会随本进程关闭。\r\n"
	// 标准流此刻不可信，直接向控制台设备写：有控制台就能看到，没有则静默跳过
	if out := consoleOut(); out != nil {
		_, _ = out.WriteString("\r\n" + msg + "\r\n")
		_ = out.Close()
	}
	logger.Error("LunarHarness", "控制台输出未连接到终端，界面不可用，进程退出")
	return ErrNoTerminal
}

// ScreenReady 返回「控制台首帧已渲染」的信号通道：宿主据此安排启动后置任务。
func ScreenReady() <-chan struct{} { return screenReadyCh }

// markScreenReady 标记控制台首帧已渲染（只关闭一次信号）。
func markScreenReady() { screenReadyOnce.Do(func() { close(screenReadyCh) }) }

// Run 启动全屏 TUI 应用（阻塞直至用户退出；宿主需已先调用 Install）
func Run() error {
	model := newAppModel()

	// 进事件循环前先确认控制台可用：否则会留下「进程在跑、界面全无」的隐形实例
	if err := ensureTerminal(); err != nil {
		return err
	}

	// 控制台编码与字形宽度：中文 Windows 的经典 conhost 默认用 GBK 代码页，且中文点阵字体
	// 会把 ◆ ■ ▶ · → 这类「东亚歧义宽度」字形渲染成 2 列。两者都会让每一行超出终端宽度，
	// 一折行整屏就翻倍高、备用屏幕放不下便每帧向上滚（用户实测：一打开就滚动 + 显示混乱）。
	// 先切 UTF-8 再实测字形宽度——顺序不能反，否则测到的是乱码的宽度。
	renderTarget := GetRealStdout()
	if f, ok := renderTarget.(*os.File); ok {
		restoreEncoding, fingerprint := ensureConsoleEncoding(f)
		defer restoreEncoding()
		widths := calibrateGlyphWidths(f)
		// 环境指纹先写入原始日志：判断「乱码 / 错位 / 滚动」时，第一手证据就是
		// 「跑在哪个终端、代码页是多少、字形实测多宽」
		if fingerprint != "" {
			pushScrubbedLog(SourceRaw, "RAW", "TUI", "", fingerprint)
		}
		if len(widths) > 0 {
			wide := make([]string, 0, 6)
			summary := make([]string, 0, 8)
			for _, g := range probeGlyphs() {
				w, ok := widths[g]
				if !ok {
					continue
				}
				SetMeasuredGlyphWidth(g, w)
				if w > 1 {
					wide = append(wide, fmt.Sprintf("%c=%d列", g, w))
				}
			}
			summary = append(summary, fmt.Sprintf("[TUI] 终端字形校准: 实测 %d 个字形", len(widths)))
			if len(wide) > 0 {
				summary = append(summary, "  比标准宽度更宽（已按终端宽度排版）: "+strings.Join(wide, " "))
			} else {
				summary = append(summary, "  无更宽字形（终端与本程序宽度表一致）")
			}
			msg := strings.Join(summary, "\n")
			pushScrubbedLog(SourceRaw, "RAW", "", "", msg)
		} else {
			pushScrubbedLog(SourceRaw, "WARN", "TUI", "", "终端字形校准失败：未能取得控制台信息，已退回内置宽度表（中文控制台字体下可能错位）")
		}
	}

	// 配色档位与样式绑定：必须在 VT 处理开启之后、tea 启动之前。
	// 这一步看着琐碎，却是「界面一片纯白」的根治点——见 renderer.go 的说明。
	// 顺序也要紧：颜色档位取决于「控制台是否已启用 ANSI 处理」，反了就永远退化为无色。
	if diag := activateTheme(renderTarget); diag != "" {
		pushScrubbedLog(SourceRaw, "RAW", "", "", "[TUI] 配色档位: "+diag)
	}

	// 输入同样不信任继承句柄：句柄不可用时改读控制台设备，否则 TUI 收不到按键
	prog := tea.NewProgram(model,
		tea.WithAltScreen(),
		tea.WithOutput(GetRealStdout()),
		tea.WithInput(resolveInputTarget()),
	)

	// 流式事件订阅（TUI 激活期消费，退出时注销）
	streamCh, unsub := SubscribeStream()
	model.streamUnsub = unsub

	// 流式事件转发 goroutine（done 通道随退出关闭）
	done := make(chan struct{})
	go func() {
		for {
			select {
			case evt := <-streamCh:
				prog.Send(streamEvtMsg{evt: evt})
			case <-done:
				return
			}
		}
	}()

	// 日志管线唤醒转发 goroutine
	go func() {
		for range notifyCh {
			prog.Send(notifyMsg{})
		}
	}()

	// 终端尺寸轮询：Windows 无 SIGWINCH，bubbletea 只在启动时取一次尺寸；
	// 窗口改小后框架仍按旧尺寸渲染整屏 → 每帧都高出一截 → 控制台不停向上滚。
	if out, ok := GetRealStdout().(*os.File); ok {
		go watchTerminalSize(prog.Send, func() (int, int, bool) { return consoleSize(out) }, done)
	}

	// 功能子包钩子注入：日志并入统一管线，进度推送到 TUI。
	taskLog := func(msg string) {
		PushTaskLog(msg)
		prog.Send(taskLogMsg(msg))
	}
	archive.SetLogHook(taskLog)
	archive.SetProgressHook(func(percent int) { prog.Send(progressMsg(percent)) })
	proxy.SetLogHook(taskLog)
	portrelease.SetLogHook(taskLog)

	// 终止信号 → 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		prog.Quit()
	}()

	_, err := prog.Run()
	close(done)
	model.quitCleanup()
	return err
}
