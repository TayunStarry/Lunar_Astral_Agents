package console

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/archive"
	"LunarSubsystem/LunarHarness/engine/console/updatepack"
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

// viewArchive 打包屏渲染
func (m appModel) viewArchive() string {
	f := m.frameFor("打包归档", archiveContext(m.archive), archiveKeys(m.archive))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.archive.running:
		body = archiveRunningBody(m, w, h)
	case m.archive.inputMode:
		body = m.archiveInputBody(w)
	case m.archive.loaded != nil:
		body = m.archiveDetailBody(w, h)
	default:
		body = m.archiveListBody(w, h)
		f.status = dimStyle.Render(posText(m.archive.cfgIdx, len(m.archive.configs), "个配置"))
	}
	body = withErrStrip(body, m.archive.err, w)
	return f.render(body)
}

// archiveRunningBody 打包进行中
func archiveRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render("正在打包归档…"),
		"",
	}
	if m.archive.progress > 0 {
		lines = append(lines, "  "+progressBar(m.archive.progress, minInt(w-8, 60)))
	} else {
		lines = append(lines, "  "+dimStyle.Render("准备中：正在扫描文件清单"))
	}
	lines = append(lines, "",
		"  "+dimStyle.Render("配置: ")+valueStyle.Render(m.archive.loadedPath),
		"  "+dimStyle.Render("详细过程见「原始日志」"))
	return fitLines(lines, h)
}

// archiveInputBody 自定义配置文件路径输入
func (m appModel) archiveInputBody(w int) []string {
	boxW := minInt(w, 78)
	body := []string{
		"",
		"  " + labelStyle.Render("输入打包配置文件路径（留空回车返回列表）："),
		"",
	}
	body = append(body, formBox([]string{"", "  " + m.archive.input.View(), ""}, boxW)...)
	body = append(body,
		"",
		"  "+dimStyle.Render("默认配置：")+valueStyle.Render(archive.DefaultConfigPath),
		"  "+dimStyle.Render("也可放在 data/*.json，重新进入本屏即可自动发现"))
	return body
}

// archiveListBody 候选配置列表（定高窗口滚动）
func (m appModel) archiveListBody(w, h int) []string {
	head := []string{
		sectionRule("候选打包配置", w),
		dimStyle.Render("  回车加载配置并进入确认；按 [p] 可手动指定配置文件路径"),
		"",
	}
	if len(m.archive.configs) == 0 {
		body := append([]string{}, head...)
		body = append(body,
			dimStyle.Render("  未找到打包配置（默认 "+archive.DefaultConfigPath+"，或 data/*.json）"),
			"",
			dimStyle.Render("  将配置文件放入上述任一位置后重新进入本屏，或按 [p] 输入其他路径"))
		return fitLines(body, h)
	}

	idx := clampInt(m.archive.cfgIdx, 0, len(m.archive.configs)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(m.archive.configs))
	for i, path := range m.archive.configs {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		name := path
		tag := ""
		if path == archive.DefaultConfigPath {
			tag = "默认"
		}
		title := labelStyle.Render("◆ " + name)
		if sel {
			title = selectStyle.Render("◆ " + name)
		}
		blocks[i] = []string{listRow(rowW, sel, cursor, title, dimStyle.Render(tag))}
	}

	lines, _ := m.archive.list.layoutView(blocks, idx, w, maxInt(1, h-len(head)))
	body := append([]string{}, head...)
	body = append(body, lines...)
	return fitLines(body, h)
}

// archiveDetailBody 已加载配置确认态：键值卡片
func (m appModel) archiveDetailBody(w, h int) []string {
	cfg := m.archive.loaded
	if cfg == nil {
		return blankLines(h, w)
	}
	keyW := 10
	rows := []string{
		sectionRule("已加载配置", w),
		"",
		kvRow("配置文件", m.archive.loadedPath, keyW),
		kvRow("输出路径", cfg.Output, keyW),
		kvRow("分卷大小", fmt.Sprintf("%d MB", cfg.Size), keyW),
		kvRow("压缩等级", fmt.Sprintf("%d", cfg.Level), keyW),
		kvRow("打包项数", fmt.Sprintf("%d", len(cfg.Import)), keyW),
	}
	if len(cfg.Exclude) > 0 {
		rows = append(rows, kvRow("排除项", strings.Join(cfg.Exclude, "  "), keyW))
	}
	if len(cfg.Import) > 0 {
		rows = append(rows, "", sectionRule("打包来源", w), "")
		room := maxInt(1, h-len(rows)-2)
		for _, item := range cfg.Import {
			if room <= 0 {
				break
			}
			rows = append(rows, "  "+labelStyle.Render(truncTo(item, maxInt(8, w-4))))
			room--
		}
	}
	rows = append(rows, "", "  "+dimStyle.Render("按 Enter 开始打包；Esc 返回配置列表"))
	return fitLines(rows, h)
}

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
// 为什么要它：含主程序的更新必须「控制台退出 → 更新器覆盖 → 重启」，控制台一退，
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
	{"1", "发布更新包", "收集本地资源 / 主程序 → 打包 → 上传 Gitee Release（需输入令牌）"},
	{"2", "拉取最新", "取最新 Release 附件并解压到应用目标目录"},
	{"3", "按 tag 拉取", "输入版本标签拉取指定发布"},
	{"4", "按哈希拉取", "输入 commit 哈希（zipball 兜底通道）"},
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

// submitUpdateInput 输入态回车提交（值接收者：修改随返回的模型副本生效）
func (m appModel) submitUpdateInput() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.update.input.Value())
	if value == "" {
		m.update.mode = updateMenuMode
		m.update.input.Blur()
		return m, nil
	}
	m.update.running = true
	m.update.progress = 0
	m.update.err = nil
	m.update.input.Blur() // 提交后离开输入态
	switch m.update.mode {
	case updateInputTag:
		m.update.runningOp = "拉取更新包（tag: " + value + "）"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("tag", value)
	case updateInputToken:
		// 令牌由发布时手动输入：配置文件不再保存密钥。
		// 同时记入会话：后续列表/拉取请求都会带上它，避免一直吃匿名额度（限流）
		SetSessionToken(value)
		m.update.runningOp = "发布更新包"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("publish", value)
	}
	m.update.runningOp = "拉取更新包（commit: " + value + "）"
	m.setBusy(m.update.runningOp)
	return m, runUpdateCmd("hash", value)
}

// updateMenuAction 执行操作菜单当前选中项
func (m appModel) updateMenuAction() (tea.Model, tea.Cmd) {
	idx := clampInt(m.update.menuIdx, 0, len(updateMenuEntries)-1)
	switch idx {
	case 0:
		// 发布需 Gitee 私人令牌：先进入令牌输入态（不回显明文）
		// 配置文件 release.json 已有令牌时预填，直接回车即可发布
		m.update.mode = updateInputToken
		m.update.input.EchoMode = textinput.EchoPassword
		m.update.input.SetValue(configuredGiteeToken())
		m.update.input.Placeholder = "输入 Gitee 私人令牌（需 Release 读写权限）"
		m.update.input.Focus() // 必须 Focus：未聚焦时 textinput 直接丢弃按键
		m.update.err = nil
		return m, textinput.Blink
	case 1:
		m.update.running = true
		m.update.progress = 0
		m.update.err = nil
		m.update.runningOp = "拉取最新更新包"
		m.setBusy(m.update.runningOp)
		return m, runUpdateCmd("latest", "")
	case 2:
		m.update.mode = updateInputTag
		m.update.input.EchoMode = textinput.EchoNormal
		m.update.input.SetValue("")
		m.update.input.Placeholder = "例如 v2026.10.14 或 v1.2.0"
		m.update.input.Focus()
		return m, textinput.Blink
	case 3:
		m.update.mode = updateInputHash
		m.update.input.EchoMode = textinput.EchoNormal
		m.update.input.SetValue("")
		m.update.input.Placeholder = "输入 commit 哈希（支持短哈希前缀）"
		m.update.input.Focus()
		return m, textinput.Blink
	default:
		// 列表缓存：短时间内重复进入直接复用，避免无谓消耗 Gitee 额度（匿名额度极低）
		if !m.update.versionsAt.IsZero() && time.Since(m.update.versionsAt) < listCacheTTL {
			m.update.mode = updateVersions
			m.update.versionIx = 0
			m.update.versionPl = listPane{}
			m.setNotice(fmt.Sprintf("复用 %d 个已发布版本（%s 内缓存；刷新请按 r）",
				len(m.update.versions), shortDuration(listCacheTTL)), false)
			return m, nil
		}
		m.update.running = true
		m.update.runningOp = "获取版本列表"
		m.setBusy(m.update.runningOp)
		return m, listVersionsCmd()
	}
}

// runUpdateCmd 后台执行更新包操作
func runUpdateCmd(kind, value string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateDoneMsg{err: fmt.Errorf("未配置发布通道（local_data/release.json 缺少仓库信息）")}
		}
		var err error
		var res updatepack.PullResult
		switch kind {
		case "publish":
			// value 为发布时手动输入的 Gitee 令牌（配置文件不保存密钥）：
			// 传入副本以免污染全局配置
			cfgCopy := *cfg
			cfgCopy.GiteeToken = value
			err = updatepack.Publish(cfgCopy, "")
		case "latest":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullLatest})
		case "tag":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullTag, Value: value})
		case "hash":
			res, err = updatepack.Pull(*withSessionTokenUpdate(cfg), updatepack.PullSpec{Kind: updatepack.PullHash, Value: value})
		}
		if err != nil {
			return updateDoneMsg{err: err}
		}
		// 含主程序的更新包：拉起更新应用器后退出 TUI，由应用器在本进程释放文件占用后覆盖并重启
		if res.NeedsRestart {
			if aerr := updatepack.ApplyProgramUpdate(res, *cfg); aerr != nil {
				return updateDoneMsg{err: aerr}
			}
			// 记录交接说明：控制台退出后打印到真实终端，避免「程序凭空消失」
			self, _ := os.Executable()
			note := &handoverNote{
				PackFile: filepath.Base(res.PackPath),
				Program:  self,
				Root:     updatepack.ProgramRoot(*cfg),
				Files:    res.ProgramFiles,
				LogPath:  updatepack.ApplyLogPath(*cfg),
				Applier:  updatepack.ApplierPath(),
			}
			return updateDoneMsg{restart: true, handover: note}
		}
		return updateDoneMsg{err: nil}
	}
}

// listVersionsCmd 后台获取版本列表。
//
// 会话令牌会一并带上（配置里有则以配置为准）；若正处于限流冷却期则不请求，
// 直接返回人话提示，避免用户反复刷新把额度耗得更久。
func listVersionsCmd() tea.Cmd {
	return func() tea.Msg {
		if wait := rateLimitRemaining(); wait > 0 {
			return updateVersionsMsg{err: fmt.Errorf("Gitee 接口限流冷却中，约 %s 后可重试", shortDuration(wait))}
		}
		cfg := GetUpdatePackConfig()
		if cfg == nil {
			return updateVersionsMsg{err: fmt.Errorf("未配置发布通道（local_data/release.json 缺少仓库信息）")}
		}
		versions, err := updatepack.ListVersions(*withSessionTokenUpdate(cfg))
		if err != nil {
			noteAPIRateLimit(err)
			return updateVersionsMsg{err: err}
		}
		clearRateLimit()
		return updateVersionsMsg{versions: versions}
	}
}

// updateContext 标题栏上下文：仓库与应用目标
func updateContext(s updateState) string {
	cfg := GetUpdatePackConfig()
	if cfg == nil || cfg.GiteeOwner == "" {
		return "未配置仓库"
	}
	ctx := cfg.GiteeOwner + "/" + cfg.GiteeRepo
	target := cfg.TargetDir
	if target == "" || target == "." {
		target = "程序根目录"
	}
	ctx += " · 应用目标 " + target
	switch s.mode {
	case updateVersions:
		ctx += " · 版本列表（r 刷新）"
	case updateInputTag:
		ctx += " · 按 tag 拉取"
	case updateInputHash:
		ctx += " · 按哈希拉取"
	case updateInputToken:
		ctx += " · 发布更新包"
	case updateHandover:
		ctx += " · 已交接更新器"
	}
	return ctx
}

// updateKeys 更新包屏键位（随模式变化）
func updateKeys(s updateState) []keyHint {
	if s.mode == updateHandover {
		return []keyHint{
			kh("Enter/Esc", "立即退出控制台（更新器已在等待）"),
		}
	}
	if s.running {
		return []keyHint{kh("…", "任务进行中，完成前不可操作")}
	}
	switch s.mode {
	case updateInputTag, updateInputHash, updateInputToken:
		return []keyHint{kh("Enter", "确认"), kh("Esc", "返回操作菜单")}
	case updateVersions:
		return []keyHint{
			kh("↑/↓ PgUp/PgDn", "选择版本"),
			kh("Enter", "按该 tag 拉取"),
			kh("r", "刷新列表"),
			kh("Esc/←", "返回操作菜单"),
		}
	}
	return []keyHint{
		kh("↑/↓", "选择操作"),
		kh("Enter", "执行"),
		kh("1-5", "快捷键直达"),
		kh("Esc", "返回菜单"),
	}
}

// viewUpdate 更新包屏渲染
func (m appModel) viewUpdate() string {
	f := m.frameFor("更新包", updateContext(m.update), updateKeys(m.update))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.update.mode == updateHandover:
		body = updateHandoverBody(m, w)
		f.status = warnStyle.Render(fmt.Sprintf("%d 秒后自动退出", m.update.handoverLeft))
	case m.update.running:
		body = updateRunningBody(m, w, h)
	case m.update.mode == updateInputTag || m.update.mode == updateInputHash || m.update.mode == updateInputToken:
		body = m.updateInputBody(w)
	case m.update.mode == updateVersions:
		body = m.updateVersionsBody(w, h)
		f.status = dimStyle.Render(posText(m.update.versionIx, len(m.update.versions), "个版本"))
	default:
		body = m.updateMenuBody(w, h)
	}
	body = withErrStrip(body, m.update.err, w)
	return f.render(body)
}

// updateHandoverBody 交接画面：把「接下来会发生什么、去哪看」讲清楚。
//
// 用户视角的真相：点了「拉取最新」之后控制台会消失（主程序 exe 被锁，必须退出才能覆盖），
// 若不给这段说明，屏幕上就只是「黑了」，无从判断是成功还是失败。
func updateHandoverBody(_ appModel, w int) []string {
	note := pendingHandover
	keyW := 12
	body := []string{
		"",
		"  " + okStyle.Render("✓ 更新包已下载，主程序更新已交给更新器接续"),
		"",
		"  " + labelStyle.Render("接下来会自动完成（无需任何操作）："),
		"    " + dimStyle.Render("① 控制台退出 → ② 释放主程序文件占用 → ③ 解压并覆盖 → ④ 原样重启程序"),
		"",
		sectionRule("本次更新", w),
		"",
	}
	if note != nil {
		body = append(body,
			kvRow("更新包", note.PackFile, keyW),
			kvRow("覆盖目标", note.Root, keyW),
			kvRow("主程序", note.Program, keyW),
			kvRow("过程日志", note.LogPath, keyW),
		)
		if len(note.Files) > 0 {
			body = append(body, kvRow("纳入文件", strings.Join(note.Files, "、"), keyW))
		}
	} else {
		body = append(body, dimStyle.Render("  （交接信息不可用，详见过程日志）"))
	}
	body = append(body,
		"",
		"  "+warnStyle.Render("请勿关闭本终端窗口：")+dimStyle.Render("更新器是它的子进程，关窗会连带终止覆盖过程。"),
		"  "+dimStyle.Render("程序会自动重启；若长时间未重启，请查看上面的过程日志。"),
	)
	return body
}

// updateRunningBody 任务进行中
func updateRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render(m.update.runningOp+"…"),
		"",
	}
	if m.update.progress > 0 {
		lines = append(lines, "  "+progressBar(m.update.progress, minInt(w-8, 60)))
	} else {
		lines = append(lines, "  "+dimStyle.Render("准备中：正在与 Gitee 建立通道"))
	}
	lines = append(lines, "", "  "+dimStyle.Render("详细过程见「原始日志」"))
	return fitLines(lines, h)
}

// updateMenuBody 操作菜单（可选中，数字键直达）
func (m appModel) updateMenuBody(w, h int) []string {
	head := []string{
		sectionRule("更新包操作", w),
		dimStyle.Render("  上下键选择后回车执行，也可直接按数字键"),
		"",
	}
	idx := clampInt(m.update.menuIdx, 0, len(updateMenuEntries)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(updateMenuEntries))
	for i, ent := range updateMenuEntries {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		title := labelStyle.Render("[" + ent.key + "] " + ent.title)
		if sel {
			title = selectStyle.Render("[" + ent.key + "] " + ent.title)
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, ""),
			newRow(rowW, sel).L("     ", dimStyle.Render(truncTo(ent.desc, maxInt(8, rowW-6)))).String(),
		}
	}
	lines, _ := m.update.menuPl.layoutView(blocks, idx, w, maxInt(1, h-len(head)-2))
	body := append([]string{}, head...)
	body = append(body, lines...)
	body = append(body, "", "  "+dimStyle.Render("含主程序的更新包会自动关闭本程序、备份覆盖并重启"))
	return fitLines(body, h)
}

// updateInputBody 输入态
func (m appModel) updateInputBody(w int) []string {
	var prompt, hint string
	switch m.update.mode {
	case updateInputTag:
		prompt = "输入要拉取的版本 tag（留空回车返回）："
		hint = "例如 v2026.10.14；对应 Gitee Release 的 tag 名"
	case updateInputHash:
		prompt = "输入要拉取的 commit 哈希（留空回车返回）："
		hint = "支持短哈希前缀；Release 通道不可用时走 zipball 兜底"
	default:
		prompt = "输入 Gitee 私人令牌（发布需 Release 读写权限，留空回车取消）："
		hint = "release.json 已保存令牌时自动预填，可直接回车发布；令牌只用于本次请求"
	}
	boxW := minInt(w, 78)
	body := []string{
		"",
		"  " + labelStyle.Render(prompt),
		"",
	}
	body = append(body, formBox([]string{"", "  " + m.update.input.View(), ""}, boxW)...)
	body = append(body, "", "  "+dimStyle.Render(hint))
	return body
}

// updateVersionsBody 版本列表（定高窗口滚动，可直接选中拉取）
func (m appModel) updateVersionsBody(w, h int) []string {
	head := []string{
		sectionRule("仓库已发布版本（仅更新包通道）", w),
		dimStyle.Render("  回车按该 tag 拉取更新包；同仓库的 LTPX 应用市场发布已自动排除"),
		"",
	}
	if len(m.update.versions) == 0 {
		body := append([]string{}, head...)
		body = append(body, dimStyle.Render("  仓库暂无发布版本"))
		return fitLines(body, h)
	}

	idx := clampInt(m.update.versionIx, 0, len(m.update.versions)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(m.update.versions))
	for i, v := range m.update.versions {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		title := labelStyle.Render("◆ " + marketOrDash(v.TagName))
		right := dimStyle.Render(marketOrDash(v.Name))
		if sel {
			title = selectStyle.Render("◆ " + marketOrDash(v.TagName))
			right = keyStyle.Render(marketOrDash(v.Name))
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, right),
			newRow(rowW, sel).L("    ", dimStyle.Render(truncTo(firstLine(v.Body), maxInt(8, rowW-6)))).String(),
		}
	}

	lines, _ := m.update.versionPl.layoutView(blocks, idx, w, maxInt(1, h-len(head)))
	body := append([]string{}, head...)
	body = append(body, lines...)
	return fitLines(body, h)
}
