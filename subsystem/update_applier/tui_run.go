package main

// ==== 更新器 TUI 启动 ====

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// RunTUI 启动更新器界面，阻塞到用户退出。
//
// startupErr 非空表示配置加载失败：界面仍会启动，让用户看到原因并进入日志屏，
// 而不是双击后窗口一闪而过。
func RunTUI(root string, cfg *ReleaseConfig, startupErr error) error {
	in, out := consoleHandles()
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	restore := ensureConsole(out)
	defer restore()
	calibrateGlyphWidths(out)

	model := newUIModel(root, cfg)
	if startupErr != nil {
		model.relErr = startupErr
		model.setNotice("配置加载失败", true)
		model.appendLog("[ERROR] " + startupErr.Error())
	}

	prevHook := logHook
	// 日志行本体写入内存环形缓冲，这里只发唤醒信号：
	// events 通道拥塞时界面仍能在下一秒补读到全部日志。
	logHook = func(string) { emit(model.events, msgLog{}) }
	defer func() { logHook = prevHook }()

	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithAltScreen(),
	)

	// 尺寸轮询：bubbletea 在 Windows 上只在启动时量一次终端尺寸（见 watchTerminalSize 注释）
	stop := make(chan struct{})
	defer close(stop)
	go watchTerminalSize(out, program.Send, stop)

	_, err := program.Run()
	return err
}

// watchTerminalSize 轮询终端尺寸，变化时向界面投递 WindowSizeMsg。
//
// 为什么必须自己轮询：bubbletea 在 Windows 上没有 SIGWINCH（signals_windows.go 的
// listenForResize 直接关闭），缩放事件只能从 coninput 事件循环里读到；而该循环只在
// 输入句柄恰好是 os.Stdin 时才启用（inputreader_windows.go 里按 Fd() 比对）。
// 本程序为保证按键可用，输入取自自开的 CONIN$ 句柄，于是 coninput 路径被绕过，
// 尺寸只剩启动时 checkResize 上报的那一次。不补轮询的后果：拖动/最大化窗口后界面
// 永远停在旧尺寸，窗口改小时整屏高于可视区，备用屏幕放不下便持续上滚且不会恢复。
func watchTerminalSize(out *os.File, send func(tea.Msg), stop <-chan struct{}) {
	lastW, lastH, ok := terminalSize(out)
	if !ok {
		return
	}
	ticker := time.NewTicker(uiSizePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			w, h, valid := terminalSize(out)
			if !valid || (w == lastW && h == lastH) {
				continue
			}
			logf("终端尺寸变化：%d×%d → %d×%d，界面按新尺寸重排", lastW, lastH, w, h)
			lastW, lastH = w, h
			// 程序已退出时 Send 会阻塞在 msgs 上，先看停止信号再投递
			select {
			case <-stop:
				return
			default:
			}
			send(tea.WindowSizeMsg{Width: w, Height: h})
		}
	}
}

// terminalSize 读取终端可视窗口尺寸（列 × 行）
func terminalSize(out *os.File) (int, int, bool) {
	if out == nil || !term.IsTerminal(out.Fd()) {
		return 0, 0, false
	}
	w, h, err := term.GetSize(out.Fd())
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return w, h, true
}

// newUIModel 构造初始模型（读版本信标与安装状态在启动时完成）
func newUIModel(root string, cfg *ReleaseConfig) *uiModel {
	m := &uiModel{
		width:     100,
		height:    32,
		root:      root,
		cfg:       cfg,
		screen:    screenOverview,
		logFollow: true,
		startAt:   time.Now(),
		events:    make(chan any, 256),
	}
	m.curVersion = currentVersion(root)
	if st, err := readState(root); err == nil {
		m.state = st
	}
	m.publish.Version = defaultTag()
	if logInitErr != nil {
		m.appendLog("[ERROR] " + logInitErr.Error())
	}
	m.appendLog("更新器已启动 · 目标 " + root)
	m.appendLog("当前版本 " + versionOrDash(m.curVersion))
	if cfg != nil {
		m.appendLog("更新通道 " + repoLabel(cfg) + " · 分卷 " +
			strconv.Itoa(packSizeMB(cfg)) + "MB · 备份保留 " + strconv.Itoa(backupKeepCount(cfg)) + " 次")
	}
	// 启动阶段（openLog / 启动恢复 / 配置加载）产生的日志在钩子安装之前，
	// 这里从环形缓冲补读，保证界面日志屏与日志文件内容同源。
	m.drainLogs()
	m.refreshProgramState()
	return m
}
