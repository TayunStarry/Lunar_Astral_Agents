package TuiSystem

// ==== LTP9 授权签发屏 ====
//
// 扫描 local_data/package 下的 LTP9 包（metadata.json 的 tags 含 LTP9），在列表中用颜色
// 呈现授权状态：
//
//	绿 已签发（现有密钥能覆盖脚本引用的权限）
//	橙 未签发 / 已签发但权限不全
//	红 需重签（密钥与当前脚本已不对应，运行时会被判为无权限）/ 无入口脚本
//	灰 无需签发（脚本只使用常驻能力）
//
// 回车进入确认态：签发范围由**脚本现有引用**推断（LTP9 的权限按顶层全局对象分布，
// 脚本调用了哪个根对象就授予哪项权限），确认后覆盖写入 permissions.key。
//
// 本屏是**纯后端 UI**：不依赖 webUI，也不依赖独立工具 subsystem/ltp9_keygen
// （那套 Web 实现保留作开发者工具）；签发与现状读取经 SetLTP9AuthHooks 由宿主注入，
// 保证与引擎的运行时校验共用同一份规则。

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"LunarSubsystem/TuiSystem/ltpxmarket"
)

// ltp9Tag LTP9 包的标签（与引擎 LTP9Tag 一致，大小写不敏感比较）
const ltp9Tag = "LTP9"

// ltp9AuthMode 授权屏操作模式
type ltp9AuthMode int

const (
	ltp9AuthBrowse  ltp9AuthMode = iota // 浏览候选包
	ltp9AuthConfirm                     // 二次确认签发
)

// ltp9AuthPkg 候选包：包元数据 + 引擎侧授权现状
type ltp9AuthPkg struct {
	Dir     string
	DirName string
	Meta    ltpxmarket.PackageMeta
	Auth    LTP9AuthStatus
	Err     error // 读取现状失败（该包不可签发）
}

// ltp9AuthState 授权屏状态
type ltp9AuthState struct {
	mode    ltp9AuthMode
	pkgs    []ltp9AuthPkg
	idx     int
	loaded  bool // 是否已成功扫描（避免每次进入都重扫）
	running bool
	op      string
	err     error
	pl      listPane
	receipt string // 最近一次签发回执（成功后展示在标题栏上下文）
}

// ltp9AuthScanMsg 候选包扫描完成
type ltp9AuthScanMsg struct {
	pkgs []ltp9AuthPkg
	err  error
}

// ltp9AuthSignMsg 签发完成
type ltp9AuthSignMsg struct {
	dir    string
	status LTP9AuthStatus
	err    error
}

// newLTP9AuthState 构建授权屏初始状态
func newLTP9AuthState() ltp9AuthState { return ltp9AuthState{} }

// ltp9AuthEnter 进入授权屏：首次进入自动扫描
func (m *appModel) ltp9AuthEnter() tea.Cmd {
	m.ltp9Auth.err = nil
	if m.ltp9Auth.loaded {
		return nil
	}
	return m.ltp9AuthRescan()
}

// ltp9AuthRescan 重新扫描候选包（进入屏与按 [r] 都走这里）
func (m *appModel) ltp9AuthRescan() tea.Cmd {
	m.ltp9Auth.running = true
	m.ltp9Auth.op = "扫描 LTP9 包"
	m.ltp9Auth.err = nil
	m.setBusy("扫描 LTP9 包")
	return scanLTP9AuthCmd()
}

// updateLTP9Auth 授权屏事件
func (m appModel) updateLTP9Auth(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ltp9AuthScanMsg:
		m.ltp9Auth.running = false
		m.ltp9Auth.pkgs = msg.pkgs
		m.ltp9Auth.idx = 0
		m.ltp9Auth.pl = listPane{}
		m.ltp9Auth.err = msg.err
		m.ltp9Auth.loaded = msg.err == nil
		if msg.err == nil {
			m.setNotice(fmt.Sprintf("扫描到 %d 个 LTP9 包", len(msg.pkgs)), false)
		} else {
			m.setNotice("LTP9 包扫描失败", true)
		}
		return m, nil

	case ltp9AuthSignMsg:
		m.ltp9Auth.running = false
		m.ltp9Auth.mode = ltp9AuthBrowse
		m.ltp9Auth.err = msg.err
		if msg.err != nil {
			m.setNotice("签发失败", true)
			return m, nil
		}
		for i := range m.ltp9Auth.pkgs {
			if m.ltp9Auth.pkgs[i].Dir == msg.dir {
				m.ltp9Auth.pkgs[i].Auth = msg.status
				m.ltp9Auth.pkgs[i].Err = nil
			}
		}
		m.ltp9Auth.receipt = fmt.Sprintf("已签发 %s：%s",
			filepath.Base(msg.dir), ltp9AuthOrDash(strings.Join(msg.status.Granted, " · ")))
		m.setNotice("签发完成（未引用的权限已收回）", false)
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.ltp9Auth.running {
				return m, nil
			}
			if m.ltp9Auth.mode == ltp9AuthConfirm {
				m.ltp9Auth.mode = ltp9AuthBrowse
				return m, nil
			}
			m.screen = screenMenu
			return m, nil
		}
		if m.ltp9Auth.running {
			return m, nil
		}
		return m.updateLTP9AuthKeys(key)
	}
	return m, nil
}

// updateLTP9AuthKeys 授权屏非提交类按键分发
func (m appModel) updateLTP9AuthKeys(key string) (tea.Model, tea.Cmd) {
	if m.ltp9Auth.mode == ltp9AuthConfirm {
		switch key {
		case "y", "enter":
			pkg := m.currentLTP9AuthPkg()
			if pkg == nil || !pkg.Auth.HasScript || len(pkg.Auth.Required) == 0 {
				return m, nil
			}
			m.ltp9Auth.running = true
			m.ltp9Auth.op = "签发 " + pkg.DirName
			m.ltp9Auth.err = nil
			m.setBusy("签发 " + pkg.DirName)
			return m, signLTP9AuthCmd(pkg.Dir)
		case "n", "left", "backspace":
			m.ltp9Auth.mode = ltp9AuthBrowse
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "enter", "right":
		pkg := m.currentLTP9AuthPkg()
		if pkg == nil {
			return m, nil
		}
		// 不可签发的两种情形（无入口脚本 / 只使用常驻能力）不进入确认态
		if !pkg.Auth.HasScript || len(pkg.Auth.Required) == 0 {
			return m, nil
		}
		m.ltp9Auth.mode = ltp9AuthConfirm
		m.ltp9Auth.receipt = ""
		m.ltp9Auth.err = nil
		return m, nil
	case "r":
		return m, m.ltp9AuthRescan()
	}
	if idx, ok := listNavKey(m.ltp9Auth.idx, navAlias(key), len(m.ltp9Auth.pkgs), m.bodyPage(), true); ok {
		m.ltp9Auth.idx = idx
	}
	return m, nil
}

// currentLTP9AuthPkg 当前选中的候选包（列表为空时返回 nil）
func (m appModel) currentLTP9AuthPkg() *ltp9AuthPkg {
	if len(m.ltp9Auth.pkgs) == 0 {
		return nil
	}
	idx := clampInt(m.ltp9Auth.idx, 0, len(m.ltp9Auth.pkgs)-1)
	return &m.ltp9Auth.pkgs[idx]
}

// ==== 后台任务 ====

// scanLTP9AuthCmd 后台扫描包目录，筛出 LTP9 包并逐个读取授权现状。
// 包目录沿用应用市场的 PackageDir 配置（未配置时默认 local_data/package）。
func scanLTP9AuthCmd() tea.Cmd {
	return func() tea.Msg {
		cfg := ltpxmarket.Config{}
		if c := GetMarketConfig(); c != nil {
			cfg = *c
		}
		locals, err := ltpxmarket.ListLocalPackages(cfg)
		if err != nil {
			return ltp9AuthScanMsg{err: err}
		}
		pkgs := make([]ltp9AuthPkg, 0, len(locals))
		unavailable := false
		for _, lp := range locals {
			if !ltp9Tagged(lp.Meta.Tags) {
				continue
			}
			item := ltp9AuthPkg{Dir: lp.Dir, DirName: lp.DirName, Meta: lp.Meta}
			status, serr := InspectLTP9Auth(lp.Dir)
			if serr != nil {
				item.Err = serr
				unavailable = true
			} else {
				item.Auth = status
			}
			pkgs = append(pkgs, item)
		}
		if unavailable {
			return ltp9AuthScanMsg{pkgs: pkgs, err: fmt.Errorf("宿主未注入 LTP9 授权能力，本屏只能查看、无法签发")}
		}
		return ltp9AuthScanMsg{pkgs: pkgs}
	}
}

// signLTP9AuthCmd 后台为指定包签发授权
func signLTP9AuthCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		status, err := SignLTP9Auth(dir)
		return ltp9AuthSignMsg{dir: dir, status: status, err: err}
	}
}

// ltp9Tagged 判断包标签是否含 LTP9（大小写不敏感）
func ltp9Tagged(tags []string) bool {
	for _, t := range tags {
		if strings.EqualFold(strings.TrimSpace(t), ltp9Tag) {
			return true
		}
	}
	return false
}

// ==== 状态与配色 ====

// ltp9AuthLabel 授权状态标签与配色（本屏的核心视觉语言）
func ltp9AuthLabel(p ltp9AuthPkg) (string, lipgloss.Style) {
	switch {
	case p.Err != nil:
		return "不可读取", errStyle
	case !p.Auth.HasScript:
		return "无入口脚本", errStyle
	case len(p.Auth.Required) == 0:
		return "无需签发", faintStyle
	case !p.Auth.HasKey:
		return "未签发", warnStyle
	case !p.Auth.Signed:
		return "需重签", errStyle
	}
	if missing := ltp9AuthMissing(p.Auth); len(missing) > 0 {
		return "已签发·缺 " + strings.Join(missing, "/"), warnStyle
	}
	return "已签发", okStyle
}

// ltp9AuthMissing 脚本引用但现有密钥未授予的权限
func ltp9AuthMissing(st LTP9AuthStatus) []string {
	granted := map[string]bool{}
	for _, p := range st.Granted {
		granted[p] = true
	}
	var missing []string
	for _, p := range st.Required {
		if !granted[p] {
			missing = append(missing, p)
		}
	}
	return missing
}

// ltp9AuthTitle 包展示名（标题优先，缺失时退回目录名）
func ltp9AuthTitle(p ltp9AuthPkg) string {
	if t := strings.TrimSpace(p.Meta.Title); t != "" {
		return t
	}
	return p.DirName
}

// ltp9AuthSubLine 列表项第二行：包标识 + 权限摘要
func ltp9AuthSubLine(p ltp9AuthPkg) string {
	id := strings.TrimSpace(p.Meta.ID)
	if id == "" {
		id = p.DirName
	}
	detail := ltp9AuthOrDash(strings.Join(p.Auth.Required, " · "))
	if p.Auth.HasScript && len(p.Auth.Required) == 0 {
		detail = "仅常驻能力"
	}
	return "    id: " + id + " · 引用: " + detail
}

// ltp9AuthOrDash 空串展示为破折号
func ltp9AuthOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// ltp9AuthShortHash 哈希前 8 位（列表/详情展示用）
func ltp9AuthShortHash(hash string) string {
	if len(hash) <= 8 {
		return ltp9AuthOrDash(hash)
	}
	return hash[:8]
}

// ==== 键位与上下文 ====

// ltp9AuthContext 标题栏上下文
func ltp9AuthContext(s ltp9AuthState) string {
	switch {
	case s.running:
		return s.op
	case s.mode == ltp9AuthConfirm:
		return "签发确认（写入 permissions.key）"
	case s.receipt != "":
		return s.receipt
	}
	return fmt.Sprintf("候选 %d 个 · 依据脚本引用的根对象签发", len(s.pkgs))
}

// ltp9AuthKeys 授权屏键位（随模式变化，底栏与帮助页同源）
func ltp9AuthKeys(s ltp9AuthState) []keyHint {
	if s.running {
		return []keyHint{kh("…", "任务进行中，完成前不可操作")}
	}
	if s.mode == ltp9AuthConfirm {
		return []keyHint{
			kh("y", "确认签发"),
			kh("n/Esc", "取消返回"),
		}
	}
	return []keyHint{
		kh("↑/↓ PgUp/PgDn", "选择包"),
		kh("Enter", "签发授权"),
		kh("r", "重新扫描"),
		kh("Esc", "返回菜单"),
	}
}

// ==== 渲染 ====

// viewLTP9Auth 授权屏渲染：统一框体 + 按模式切换内容区
func (m appModel) viewLTP9Auth() string {
	f := m.frameFor("LTP9 授权签发", ltp9AuthContext(m.ltp9Auth), ltp9AuthKeys(m.ltp9Auth))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.ltp9Auth.running:
		body = ltp9AuthRunningBody(m, w, h)
	case m.ltp9Auth.mode == ltp9AuthConfirm:
		body = m.ltp9AuthConfirmBody(w, h)
	default:
		body = m.ltp9AuthListBody(w, h)
	}
	body = withErrStrip(body, friendlyErr(m.ltp9Auth.err), w)

	if n := len(m.ltp9Auth.pkgs); n > 0 {
		f.status = dimStyle.Render(posText(m.ltp9Auth.idx, n, "个 LTP9 包"))
	}
	return f.render(body)
}

// ltp9AuthRunningBody 任务进行中：spinner + 任务名
func ltp9AuthRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render(m.ltp9Auth.op+"…"),
		"",
		"  " + dimStyle.Render("签发会覆盖写入该包的 permissions.key（详细过程见「原始日志」）"),
	}
	return fitLines(lines, h)
}

// ltp9AuthListBody 候选包列表：宽屏两栏（列表 + 权限详情），窄屏单栏
func (m appModel) ltp9AuthListBody(w, h int) []string {
	pkgs := m.ltp9Auth.pkgs
	if len(pkgs) == 0 {
		return centerBlock([]string{
			"",
			"",
			dimStyle.Render("包目录中未发现 LTP9 包（metadata.json 的 tags 需含 LTP9）"),
			"",
			dimStyle.Render("候选途经：应用市场 [m] 安装 LTP9 工具包后回到本屏按 [r] 重新扫描"),
		}, w)
	}

	idx := clampInt(m.ltp9Auth.idx, 0, len(pkgs)-1)
	twoPane := w >= 88
	listW, detailW := w, 0
	if twoPane {
		listW = clampInt(w*52/100, 40, 60)
		detailW = w - listW - 4 // 竖分隔 3 列 + 右栏尾部留白 1 列
	}
	rowW := maxInt(8, listW-listGutter)

	blocks := make([][]string, len(pkgs))
	for i, p := range pkgs {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		label, style := ltp9AuthLabel(p)
		titleText := "◆ " + ltp9AuthTitle(p)
		title := labelStyle.Render(titleText)
		if sel {
			title = selectStyle.Render(titleText)
		}
		sub := ltp9AuthSubLine(p)
		if sel {
			sub = selDimStyle.Render(sub)
		} else {
			sub = dimStyle.Render(sub)
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, style.Render(label)),
			truncTo(sub, rowW),
		}
	}

	lines, _ := m.ltp9Auth.pl.layoutView(blocks, idx, listW, h)
	if !twoPane {
		return lines
	}
	preview := ltp9AuthPreview(pkgs[idx], detailW, h)
	body := make([]string, h)
	for i := 0; i < h; i++ {
		body[i] = lines[i] + ruleStyle.Render(" │ ") + preview[i] + " "
	}
	return body
}

// ltp9AuthPreview 右侧详情：状态 + 引用权限（含命中依据）+ 现有密钥授予情况
func ltp9AuthPreview(p ltp9AuthPkg, w, h int) []string {
	if w <= 0 {
		return blankLines(h, w)
	}
	label, _ := ltp9AuthLabel(p)
	lines := []string{
		selectStyle.Render("◆ " + ltp9AuthTitle(p)),
		dimStyle.Render(p.DirName),
		"",
		kvRow("状态", label, 6),
		kvRow("脚本哈希", ltp9AuthShortHash(p.Auth.Hash), 6),
		"",
		sectionRule("脚本引用的权限", w),
	}
	if len(p.Auth.Required) == 0 {
		lines = append(lines, "  "+dimStyle.Render("（只使用常驻能力，无需签发）"))
	}
	for _, perm := range p.Auth.Required {
		lines = append(lines, "  "+okStyle.Render("● "+perm)+
			dimStyle.Render("  ← "+ltp9AuthOrDash(strings.Join(p.Auth.Reasons[perm], " · "))))
	}
	if p.Auth.HasKey {
		lines = append(lines, "", sectionRule("现有密钥已授予", w))
		for _, l := range wrapText(strings.Join(p.Auth.Granted, " · "), maxInt(8, w-2), 4) {
			lines = append(lines, "  "+dimStyle.Render(l))
		}
	}
	lines = append(lines, "", dimStyle.Render("  Enter 签发 · r 重新扫描"))
	return fitLines(lines, h)
}

// ltp9AuthConfirmBody 确认态：列出将写入的权限与命中依据，再等 [y] 确认
func (m appModel) ltp9AuthConfirmBody(w, h int) []string {
	p := m.currentLTP9AuthPkg()
	if p == nil {
		return blankLines(h, w)
	}
	label, _ := ltp9AuthLabel(*p)
	body := []string{
		sectionRule("确认签发授权", w),
		"",
		kvRow("目标包", ltp9AuthTitle(*p), 8),
		kvRow("目录", p.DirName, 8),
		kvRow("脚本哈希", ltp9AuthShortHash(p.Auth.Hash), 8),
		kvRow("当前状态", label, 8),
		"",
		labelStyle.Render("  将写入 permissions.key 的权限（依据脚本现有引用）："),
	}
	for _, perm := range p.Auth.Required {
		body = append(body, "    "+okStyle.Render("● "+perm)+
			dimStyle.Render("  ← "+ltp9AuthOrDash(strings.Join(p.Auth.Reasons[perm], " · "))))
	}
	if p.Auth.HasKey {
		body = append(body,
			"",
			dimStyle.Render("  当前密钥已授予：")+wrapOneline(p.Auth.Granted, w-18),
			dimStyle.Render("  签发将按上述清单覆盖旧密钥（脚本未引用的权限会被收回）"),
		)
	}
	notes := []string{
		"签发范围按「脚本调用了哪个根对象」推断：file/image/video → allow-file，http/network/fetch → allow-network …",
		"预留权限 allow-send / allow-socket 永不自动签发。",
	}
	for _, n := range notes {
		for i, l := range wrapPlain(n, maxInt(8, w-4)) {
			if i == 0 {
				body = append(body, "", "  "+dimStyle.Render(l))
			} else {
				body = append(body, "  "+dimStyle.Render(l))
			}
		}
	}
	body = append(body, "",
		"  "+keyStyle.Render("[y]")+hintStyle.Render(" 确认签发")+
			hintStyle.Render("      ")+keyStyle.Render("[n]")+hintStyle.Render(" 取消返回"))
	return fitLines(body, h)
}

// wrapOneline 权限列表压成单行（超出宽度截断）
func wrapOneline(perms []string, w int) string {
	return valueStyle.Render(truncTo(ltp9AuthOrDash(strings.Join(perms, " · ")), maxInt(8, w)))
}
