package main

// ==== 更新器 TUI 启动 ====

import (
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	_, err := program.Run()
	return err
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
