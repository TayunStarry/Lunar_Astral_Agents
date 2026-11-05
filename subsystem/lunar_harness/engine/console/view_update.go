package console

// view_update.go 更新包屏的状态、事件与消息类型。

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/updatepack"
)

// ==== 更新包屏 ====

// updateMode 更新包屏的操作模式
type updateMode int

const (
	updateMenuMode   updateMode = iota // 操作菜单
	updateInputTag                     // 输入 tag
	updateInputHash                    // 输入 commit 哈希
	updateInputToken                   // 输入 Gitee 令牌（发布更新包）
	updateVersions                     // 版本列表展示
	updateHandover                     // 主程序更新交接（更新器已拉起，倒计时后退出控制台）
)

// handoverSeconds 交接倒计时秒数：够用户读完关键信息，又不至于让更新器干等
const handoverSeconds = 6

// handoverNote 主程序更新交接说明。
//
// 为什么要它：更新包必然含主程序，覆盖必须「控制台退出 → 更新器覆盖 → 重启」，控制台一退，
// 屏幕就只剩一个空终端——用户看到的完全是黑盒（本次故障正是如此）。
// 因此交接信息落两处：屏上倒计时（退出前）+ 退出后打印到真实终端（可回看、可复制）。
type handoverNote struct {
	PackFile string   // 更新包起始分卷文件名
	Program  string   // 主程序可执行文件路径
	Root     string   // 覆盖目标（程序根目录）
	Files    []string // 本次纳入包、需等待释放的主程序文件
	LogPath  string   // 更新器过程日志
	Applier  string   // 更新器路径
}

// pendingHandover 退出 TUI 后要打印到终端的交接说明（nil 表示普通退出）。
// 由 Update 在事件循环内写入、Run 在 prog.Run() 返回后读取：同一 goroutine，
// 且不经过任何后台命令 goroutine，因此无需加锁也不会与渲染竞态。
var pendingHandover *handoverNote

// handoverTickMsg 交接倒计时的一次心跳
type handoverTickMsg struct{}

// handoverTick 生成一次倒计时心跳
func handoverTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return handoverTickMsg{} })
}

// updateMenuEntries 操作菜单条目（键位、标题、说明）
var updateMenuEntries = []struct {
	key   string
	title string
	desc  string
}{
	{"1", "发布更新包", "收集主程序与指定文件夹 → 打包 → 上传 Gitee Release（需输入令牌）"},
	{"2", "拉取最新", "下载最新更新包，关闭本程序后由更新器备份覆盖并重启"},
	{"3", "按 tag 拉取", "输入版本标签拉取指定更新包"},
	{"4", "按哈希拉取", "输入 commit 哈希（需该提交已发布更新包标签）"},
	{"5", "版本列表", "查看仓库已发布的更新包，可选中后直接拉取"},
}

// updateState 更新包屏状态
type updateState struct {
	mode      updateMode
	input     textinput.Model
	running   bool
	runningOp string
	progress  int
	err       error
	versions  []updatepack.Version
	// versionsAt 版本列表获取时刻：listCacheTTL 内重复进入直接复用，省下 Gitee 额度
	versionsAt time.Time
	menuIdx    int      // 操作菜单选中项
	menuPl     listPane // 菜单滚动窗口
	versionPl  listPane // 版本列表滚动窗口
	versionIx  int      // 版本列表选中项
	// handoverLeft 交接倒计时剩余秒数（updateHandover 模式）
	handoverLeft int
	activated    bool
}

// newUpdateState 构建更新包屏状态
func newUpdateState() updateState {
	s := updateState{}
	s.input = textinput.New()
	s.input.Width = 44
	s.input.CharLimit = 64
	return s
}

// updateEnter 进入更新包屏
func (m *appModel) updateEnter() tea.Cmd {
	m.update.mode = updateMenuMode
	m.update.err = nil
	m.update.menuPl = listPane{}
	m.update.input.Blur() // 离开输入态：未聚焦即丢弃按键，避免残焦
	m.update.input.Width = clampInt(m.width-32, 24, 64)
	return nil
}

// updateDoneMsg 更新包任务完成
type updateDoneMsg struct {
	err error
	// restart 为真表示更新包含主程序：应用器已拉起，需退出 TUI 以释放文件占用并触发重启
	restart bool
	// handover 交接说明（restart 为真时携带）。由命令 goroutine 生成、随消息回传，
	// 在事件循环内才写入包级变量——避免命令 goroutine 与渲染并发访问同一变量。
	handover *handoverNote
}

// updateVersionsMsg 版本列表获取完成
type updateVersionsMsg struct {
	versions []updatepack.Version
	err      error
}

// updateUpdate 更新包屏事件
func (m appModel) updateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case updateDoneMsg:
		m.update.running = false
		m.update.progress = 100
		m.update.err = msg.err
		if msg.err == nil {
			if msg.restart {
				// 主程序更新已交由更新器接续：先给出交接画面与倒计时，再退出控制台
				// （直接 tea.Quit 会让终端瞬间变空，用户完全不知道发生了什么）
				pendingHandover = msg.handover
				m.update.mode = updateHandover
				m.update.handoverLeft = handoverSeconds
				m.setNotice("已拉起更新器，等待控制台退出后接管", false)
				return m, handoverTick()
			}
			m.update.mode = updateMenuMode
			m.setNotice("更新包任务完成", false)
		} else {
			m.setNotice("更新包任务失败", true)
		}
		return m, nil

	case updateVersionsMsg:
		m.update.running = false
		m.update.err = msg.err
		if msg.err == nil {
			m.update.mode = updateVersions
			m.update.versions = msg.versions
			m.update.versionsAt = time.Now()
			m.update.versionIx = 0
			m.update.versionPl = listPane{}
			m.setNotice(fmt.Sprintf("读取到 %d 个已发布版本", len(msg.versions)), false)
		} else {
			// 人话提示：限流要引导填令牌 / 稍后重试，而不是把 HTTP 原文糊上去
			m.setNotice(apiFriendlyError(msg.err), true)
		}
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		// 交接态：只等倒计时或用户主动立即退出，其余按键一律吞掉
		if m.update.mode == updateHandover {
			switch key {
			case "ctrl+c", "esc", "enter", "q":
				return m, tea.Quit
			}
			return m, nil
		}
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.update.running {
				return m, nil
			}
			if m.update.mode != updateMenuMode {
				m.update.mode = updateMenuMode
				m.update.input.Blur() // 退出输入态：避免焦点残留
				return m, nil
			}
			m.screen = screenMenu
			return m, nil
		}

		if m.update.running {
			return m, nil
		}

		switch m.update.mode {
		case updateInputTag, updateInputHash, updateInputToken:
			if key == "enter" {
				return m.submitUpdateInput()
			}
			var cmd tea.Cmd
			m.update.input, cmd = m.update.input.Update(msg)
			return m, cmd

		case updateMenuMode:
			// 数字键直达对应操作
			for i, ent := range updateMenuEntries {
				if key == ent.key {
					m.update.menuIdx = i
					return m.updateMenuAction()
				}
			}
			switch key {
			case "enter":
				return m.updateMenuAction()
			}
			if idx, ok := listNavKey(m.update.menuIdx, navAlias(key), len(updateMenuEntries), len(updateMenuEntries), true); ok {
				m.update.menuIdx = idx
			}

		case updateVersions:
			switch key {
			case "enter":
				if len(m.update.versions) == 0 {
					return m, nil
				}
				idx := clampInt(m.update.versionIx, 0, len(m.update.versions)-1)
				tag := m.update.versions[idx].TagName
				m.update.running = true
				m.update.progress = 0
				m.update.err = nil
				m.update.runningOp = "拉取更新包（tag: " + tag + "）"
				m.setBusy(m.update.runningOp)
				return m, runUpdateCmd("tag", tag)
			case "left", "backspace":
				m.update.mode = updateMenuMode
				return m, nil
			case "r":
				// 手动刷新：绕过 listCacheTTL 复用，重新请求
				m.update.versionsAt = time.Time{}
				m.update.running = true
				m.update.runningOp = "刷新版本列表"
				m.setBusy(m.update.runningOp)
				return m, listVersionsCmd()
			}
			if idx, ok := listNavKey(m.update.versionIx, key, len(m.update.versions), m.bodyPage(), true); ok {
				m.update.versionIx = idx
			}
		}
	}
	return m, nil
}
