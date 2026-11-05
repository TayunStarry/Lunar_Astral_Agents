package console

// view_archive.go 打包归档屏的状态、事件与入口聚合。
// 视图渲染见 view_archive_body.go，更新包屏见 view_update*.go。

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/archive"
)

// ==== 打包归档屏 ====

// archiveState 打包归档屏状态
type archiveState struct {
	configs    []string        // 候选配置文件列表（默认配置在首）
	cfgIdx     int             // 当前选中配置
	list       listPane        // 配置列表滚动窗口
	loaded     *archive.Config // 已加载配置（进入确认态）
	loadedPath string
	running    bool
	progress   int
	err        error

	inputMode bool            // 自定义配置路径输入态
	input     textinput.Model // 自定义配置路径输入框
}

// newArchiveState 构建打包屏状态
func newArchiveState() archiveState {
	s := archiveState{}
	s.input = textinput.New()
	s.input.Placeholder = archive.DefaultConfigPath
	s.input.Width = 56
	s.input.CharLimit = 200
	return s
}

// archiveEnter 进入打包屏：每次进入重新扫描候选配置
// （配置文件可能在程序运行期间才被放入，重新进入即应看到）
func (m *appModel) archiveEnter() tea.Cmd {
	m.archive.configs = archive.ListConfigs()
	if m.archive.cfgIdx >= len(m.archive.configs) {
		m.archive.cfgIdx = 0
	}
	m.archive.list = listPane{}
	m.archive.inputMode = false
	m.archive.loaded = nil
	m.archive.err = nil
	m.archive.input.Width = clampInt(m.width-32, 24, 72)
	return nil
}

// archiveDoneMsg 打包任务完成
type archiveDoneMsg struct{ err error }

// updateArchive 打包屏事件
func (m appModel) updateArchive(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case archiveDoneMsg:
		m.archive.running = false
		m.archive.progress = 100
		m.archive.err = msg.err
		if msg.err != nil {
			m.setNotice("打包失败", true)
		} else {
			m.setNotice("打包归档完成", false)
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.archive.running {
				return m, nil
			}
			if m.archive.inputMode {
				// 输入态 Esc → 回列表
				m.archive.leaveArchiveInput()
				return m, nil
			}
			// 确认态 Esc → 回列表
			m.archive.loaded = nil
			m.screen = screenMenu
			return m, nil
		}

		if m.archive.running {
			return m, nil
		}

		// 自定义配置路径输入态
		if m.archive.inputMode {
			if key == "enter" {
				return m.loadArchiveConfig(strings.TrimSpace(m.archive.input.Value()))
			}
			var cmd tea.Cmd
			m.archive.input, cmd = m.archive.input.Update(msg)
			return m, cmd
		}

		if m.archive.loaded != nil {
			// 确认态
			switch key {
			case "enter":
				m.archive.running = true
				m.archive.progress = 0
				m.archive.err = nil
				m.setBusy("打包归档")
				return m, runArchiveCmd(m.archive.loadedPath, m.archive.loaded)
			case "left":
				m.archive.loaded = nil // 返回列表
			}
			return m, nil
		}

		// 列表态
		switch key {
		case "p":
			// 进入自定义配置文件路径输入态
			m.archive.inputMode = true
			m.archive.input.SetValue("")
			m.archive.input.Focus()
			m.archive.err = nil
			return m, textinput.Blink
		case "enter":
			if len(m.archive.configs) == 0 {
				return m, nil
			}
			return m.loadArchiveConfig(m.archive.configs[m.archive.cfgIdx])
		}
		if idx, ok := listNavKey(m.archive.cfgIdx, navAlias(key), len(m.archive.configs), m.bodyPage(), true); ok {
			m.archive.cfgIdx = idx
		}
	}
	return m, nil
}

// loadArchiveConfig 加载指定配置文件（路径为空视为取消，回到列表态）。
// 值接收者：返回的模型类型必须与 appModel 一致——若返回指针，
// 模型动态类型会在会话中途改变，类型断言与后续状态迁移都随之失稳。
func (m appModel) loadArchiveConfig(path string) (tea.Model, tea.Cmd) {
	if path == "" {
		m.archive.leaveArchiveInput()
		return m, nil
	}
	cfg, err := archive.LoadConfig(path)
	if err != nil {
		m.archive.err = err
		return m, nil
	}
	m.archive.leaveArchiveInput()
	m.archive.loaded = cfg
	m.archive.loadedPath = path
	m.archive.err = nil
	m.setNotice("已加载打包配置", false)
	return m, nil
}

// leaveArchiveInput 退出自定义路径输入态
func (s *archiveState) leaveArchiveInput() {
	s.inputMode = false
	s.input.Blur()
}

// runArchiveCmd 后台执行打包
func runArchiveCmd(path string, cfg *archive.Config) tea.Cmd {
	return func() tea.Msg {
		return archiveDoneMsg{err: archive.Execute(path, cfg)}
	}
}

// archiveContext 标题栏上下文
func archiveContext(s archiveState) string {
	switch {
	case s.running:
		return "打包中"
	case s.loaded != nil:
		return s.loadedPath
	case s.inputMode:
		return "指定配置文件"
	}
	return fmt.Sprintf("%d 个候选配置", len(s.configs))
}

// archiveKeys 打包屏键位（随子状态变化）
func archiveKeys(s archiveState) []keyHint {
	switch {
	case s.running:
		return []keyHint{kh("…", "打包进行中，完成前不可操作")}
	case s.inputMode:
		return []keyHint{kh("Enter", "加载配置"), kh("Esc", "返回列表")}
	case s.loaded != nil:
		return []keyHint{kh("Enter", "开始打包"), kh("Esc/←", "返回列表")}
	}
	return []keyHint{
		kh("↑/↓ PgUp/PgDn", "选择配置"),
		kh("Enter", "加载配置"),
		kh("p", "指定其他配置文件路径"),
		kh("Esc", "返回菜单"),
	}
}
