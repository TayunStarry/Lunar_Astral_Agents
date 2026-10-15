package TuiSystem

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/TuiSystem/ltpxmarket"
)

// ==== 应用市场屏 ====

// marketMode 应用市场屏的操作模式
type marketMode int

const (
	marketBrowseMode   marketMode = iota // 远程市场列表（浏览 / 筛选 / 安装入口）
	marketInputTag                       // 标签筛选输入（走远端接口）
	marketInputQuery                     // 关键词搜索输入（本地过滤，不触网）
	marketDetailMode                     // 包详情（描述全文 + 安装入口）
	marketSelectPkg                      // 发布第一步：选择本地包
	marketInputArchive                   // 发布第一步（备选）：输入已打包 .ltpx 文件路径
	marketInputDesc                      // 发布第二步：输入描述（#标签 识别）
	marketInputToken                     // 发布第三步：输入 Gitee 令牌
)

// marketPublishSteps 发布向导步骤（步骤条渲染数据源）
var marketPublishSteps = []string{"选择本地包", "填写描述", "输入令牌"}

// marketState 应用市场屏状态
type marketState struct {
	mode      marketMode
	input     textinput.Model
	running   bool
	runningOp string
	progress  int
	err       error

	items    []ltpxmarket.MarketItem // 远程市场列表
	itemIdx  int                     // 当前选中项（下标作用于筛选后的可见列表）
	tagHint  string                  // 当前生效的标签筛选（展示用）
	query    string                  // 关键词搜索（本地过滤）
	loaded   bool                    // 是否已成功加载过列表（避免重复进入反复拉取）
	browsePl listPane                // 市场列表滚动窗口
	detailPl textPane                // 详情描述滚动窗格

	locals   []ltpxmarket.LocalPackage // 本地包（发布候选）
	localIdx int                       // 本地包选中项
	pkgIdx   int                       // 发布流程已选中的本地包下标
	pkgPl    listPane                  // 本地包列表滚动窗口
	desc     string                    // 发布描述（转存至令牌输入前）

	archivePath string                  // 发布源为已打包 .ltpx 文件（非空时发布走文件通道）
	archivePkg  ltpxmarket.LocalPackage // 文件包元数据（展示用，ReadArchivePackage 结果）
}

// newMarketState 构建市场屏状态
func newMarketState() marketState {
	s := marketState{}
	s.input = textinput.New()
	s.input.Width = 48
	s.input.CharLimit = 500 // 描述文本可较长；标签/令牌输入共用无碍
	return s
}

// marketEnter 进入市场屏：首次进入自动拉取市场列表
func (m *appModel) marketEnter() tea.Cmd {
	m.market.err = nil
	m.market.input.Blur() // 离开输入态：未聚焦即丢弃按键，避免残焦
	m.market.input.Width = clampInt(m.width-32, 24, 72)
	if !m.market.loaded {
		m.market.running = true
		m.market.runningOp = "获取市场列表"
		m.setBusy("获取市场列表")
		return listMarketCmd(m.market.tagHint)
	}
	return nil
}

// marketVisible 应用关键词筛选后的可见市场列表
// （筛选为空时原样返回，保证既有下标语义不变）
func (m appModel) marketVisible() []ltpxmarket.MarketItem {
	q := strings.ToLower(strings.TrimSpace(m.market.query))
	if q == "" {
		return m.market.items
	}
	out := make([]ltpxmarket.MarketItem, 0, len(m.market.items))
	for _, it := range m.market.items {
		hay := strings.ToLower(it.Title + " " + it.TagName + " " + strings.Join(it.Tags, " ") + " " + it.Description)
		if strings.Contains(hay, q) {
			out = append(out, it)
		}
	}
	return out
}

// marketListMsg 市场列表获取完成
type marketListMsg struct {
	items []ltpxmarket.MarketItem
	err   error
}

// marketLocalsMsg 本地包扫描完成（发布入口）
type marketLocalsMsg struct {
	items []ltpxmarket.LocalPackage
	err   error
}

// marketArchiveMsg 已打包 .ltpx 文件元数据读取完成（发布入口备选）
type marketArchiveMsg struct {
	pkg ltpxmarket.LocalPackage
	err error
}

// marketDoneMsg 市场任务完成（发布/安装）
type marketDoneMsg struct {
	op  string
	err error
}

// updateMarket 市场屏事件
func (m appModel) updateMarket(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case marketListMsg:
		m.market.running = false
		m.market.err = msg.err
		if msg.err == nil {
			m.market.items = msg.items
			m.market.itemIdx = 0
			m.market.browsePl = listPane{}
			m.market.loaded = true
			m.market.mode = marketBrowseMode
			m.market.input.Blur()
			m.setNotice(fmt.Sprintf("市场列表已更新：%d 个包", len(msg.items)), false)
		} else {
			m.setNotice("市场列表获取失败", true)
		}
		return m, nil

	case marketLocalsMsg:
		m.market.running = false
		m.market.err = msg.err
		if msg.err == nil {
			m.market.locals = msg.items
			m.market.localIdx = 0
			m.market.pkgPl = listPane{}
			m.market.mode = marketSelectPkg
			m.setNotice(fmt.Sprintf("扫描到 %d 个本地包", len(msg.items)), false)
		} else {
			m.setNotice("本地包扫描失败", true)
		}
		return m, nil

	case marketArchiveMsg:
		m.market.running = false
		m.market.err = msg.err
		if msg.err == nil {
			// 读取成功 → 记录文件发布源，进入描述输入
			m.market.archivePath = msg.pkg.Dir
			m.market.archivePkg = msg.pkg
			m.market.mode = marketInputDesc
			m.market.input.EchoMode = textinput.EchoNormal
			m.market.input.SetValue("")
			m.market.input.Placeholder = "输入发布描述（可含 #标签，如：一个天气查询工具 #实用工具）"
			m.market.input.Focus()
			m.setNotice("已读取安装包元数据："+marketOrDash(msg.pkg.Meta.ID), false)
			return m, textinput.Blink
		}
		m.setNotice("读取安装包失败", true)
		return m, nil

	case marketDoneMsg:
		m.market.running = false
		m.market.progress = 100
		m.market.err = msg.err
		if msg.err == nil {
			m.setNotice(msg.op+" 完成", false)
			// 发布/安装成功 → 回列表并刷新（反映最新市场状态）
			m.market.mode = marketBrowseMode
			m.market.input.Blur()
			m.market.running = true
			m.market.runningOp = "刷新市场列表"
			m.setBusy("刷新市场列表")
			return m, listMarketCmd(m.market.tagHint)
		}
		m.setNotice(msg.op+" 失败", true)
		return m, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.market.running {
				return m, nil
			}
			switch m.market.mode {
			case marketInputTag, marketInputQuery, marketDetailMode, marketSelectPkg:
				m.market.mode = marketBrowseMode
				m.market.input.Blur()
				return m, nil
			case marketInputArchive, marketInputDesc, marketInputToken:
				// 发布中途 Esc → 回包选择（令牌态回退时恢复明文模式供下次输入）
				m.market.mode = marketSelectPkg
				m.market.input.Blur()
				m.market.input.EchoMode = textinput.EchoNormal
				return m, nil
			default:
				m.screen = screenMenu
				return m, nil
			}
		}

		if m.market.running {
			return m, nil
		}
		return m.updateMarketKeys(key, msg)
	}
	return m, nil
}

// updateMarketKeys 市场屏非提交类按键分发
func (m appModel) updateMarketKeys(key string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.market.mode {
	case marketInputTag, marketInputQuery, marketInputArchive, marketInputDesc, marketInputToken:
		if key == "enter" {
			return m.marketSubmit()
		}
		var cmd tea.Cmd
		m.market.input, cmd = m.market.input.Update(msg)
		return m, cmd

	case marketBrowseMode:
		items := m.marketVisible()
		switch key {
		case "enter":
			if len(items) == 0 {
				return m, nil
			}
			m.market.mode = marketDetailMode
			m.market.detailPl = textPane{}
			return m, nil
		case "i":
			return m.marketInstall()
		case "t":
			m.market.mode = marketInputTag
			m.market.input.EchoMode = textinput.EchoNormal
			m.market.input.SetValue("")
			m.market.input.Placeholder = "输入标签筛选（如 Node-LTP，留空显示全部）"
			m.market.input.Focus() // 必须 Focus：未聚焦时 textinput 直接丢弃按键
			m.market.err = nil
			return m, textinput.Blink
		case "/":
			m.market.mode = marketInputQuery
			m.market.input.EchoMode = textinput.EchoNormal
			m.market.input.SetValue(m.market.query)
			m.market.input.Placeholder = "输入关键词搜索（名称 / 标签 / 描述，留空清除）"
			m.market.input.Focus()
			m.market.err = nil
			return m, textinput.Blink
		case "u":
			m.market.running = true
			m.market.runningOp = "扫描本地包"
			m.market.err = nil
			m.setBusy("扫描本地包")
			return m, scanLocalsCmd()
		case "r":
			m.market.running = true
			m.market.runningOp = "刷新市场列表"
			m.market.err = nil
			m.setBusy("刷新市场列表")
			return m, listMarketCmd(m.market.tagHint)
		}
		idx, ok := listNavKey(m.market.itemIdx, navAlias(key), len(items), m.bodyPage(), true)
		if ok {
			m.market.itemIdx = idx
		}
		return m, nil

	case marketDetailMode:
		switch key {
		case "enter", "i":
			return m.marketInstall()
		case "left", "backspace":
			m.market.mode = marketBrowseMode
			return m, nil
		}
		if m.market.detailPl.key(key, m.bodyPage()) {
			return m, nil
		}

	case marketSelectPkg:
		switch key {
		case "enter":
			if len(m.market.locals) == 0 {
				return m, nil
			}
			// 选择目录发布源：清空文件发布源（两者互斥，后选生效）
			m.market.archivePath = ""
			m.market.archivePkg = ltpxmarket.LocalPackage{}
			m.market.pkgIdx = m.market.localIdx
			m.market.mode = marketInputDesc
			m.market.input.EchoMode = textinput.EchoNormal
			m.market.input.SetValue("")
			m.market.input.Placeholder = "输入发布描述（可含 #标签，如：一个天气查询工具 #实用工具）"
			m.market.input.Focus()
			m.market.err = nil
			return m, textinput.Blink
		case "f":
			// 上传已打包文件（琉璃导出的 .ltpx）：进入文件路径输入态
			m.market.mode = marketInputArchive
			m.market.input.EchoMode = textinput.EchoNormal
			m.market.input.SetValue("")
			m.market.input.Placeholder = "输入 .ltpx 文件路径（琉璃导出的安装包，.zip 亦可）"
			m.market.input.Focus()
			m.market.err = nil
			return m, textinput.Blink
		}
		if idx, ok := listNavKey(m.market.localIdx, navAlias(key), len(m.market.locals), m.bodyPage(), true); ok {
			m.market.localIdx = idx
		}
	}
	return m, nil
}

// marketInstall 安装当前选中的市场包
func (m appModel) marketInstall() (tea.Model, tea.Cmd) {
	items := m.marketVisible()
	if len(items) == 0 || m.market.itemIdx >= len(items) {
		return m, nil
	}
	item := items[m.market.itemIdx]
	if item.AssetURL == "" {
		m.market.err = fmt.Errorf("该包没有可下载的安装附件")
		return m, nil
	}
	m.market.running = true
	m.market.progress = 0
	m.market.err = nil
	m.market.runningOp = "安装 " + item.TagName
	m.setBusy("安装 " + marketOrDash(item.Title))
	return m, installMarketCmd(item)
}

// marketSubmit 输入态回车提交：按当前模式推进流程
func (m appModel) marketSubmit() (tea.Model, tea.Cmd) {
	value := strings.TrimSpace(m.market.input.Value())
	switch m.market.mode {
	case marketInputTag:
		m.market.input.Blur()
		m.market.tagHint = value
		m.market.mode = marketBrowseMode
		m.market.running = true
		m.market.runningOp = "获取市场列表"
		if value != "" {
			m.market.runningOp += "（标签: " + value + "）"
		}
		m.setBusy(m.market.runningOp)
		return m, listMarketCmd(value)

	case marketInputQuery:
		m.market.input.Blur()
		m.market.query = value
		m.market.itemIdx = 0
		m.market.browsePl = listPane{}
		m.market.mode = marketBrowseMode
		if value == "" {
			m.setNotice("已清除搜索", false)
		} else {
			m.setNotice("搜索："+value, false)
		}
		return m, nil

	case marketInputArchive:
		m.market.input.Blur()
		if value == "" {
			// 留空取消 → 回包选择
			m.market.mode = marketSelectPkg
			return m, nil
		}
		m.market.mode = marketBrowseMode
		m.market.running = true
		m.market.runningOp = "读取安装包元数据"
		m.market.err = nil
		m.setBusy("读取安装包元数据")
		return m, scanArchiveCmd(value)

	case marketInputDesc:
		// 描述允许为空（发布时回退 metadata.json 的 description）
		m.market.desc = value
		m.market.input.Blur()
		m.market.mode = marketInputToken
		m.market.input.EchoMode = textinput.EchoPassword
		m.market.input.SetValue("")
		m.market.input.Placeholder = "输入 Gitee 私人令牌（需 Release 读写权限）"
		m.market.input.Focus()
		return m, textinput.Blink

	case marketInputToken:
		m.market.input.EchoMode = textinput.EchoNormal
		m.market.input.Blur()
		if value == "" {
			// 留空取消 → 回包选择
			m.market.mode = marketSelectPkg
			return m, nil
		}
		// 按发布源分发：已打包文件通道 / 本地包目录通道
		if m.market.archivePath != "" {
			path := m.market.archivePath
			m.market.mode = marketBrowseMode
			m.market.running = true
			m.market.progress = 0
			m.market.err = nil
			m.market.runningOp = "发布 " + marketOrDash(m.market.archivePkg.Meta.ID)
			m.setBusy(m.market.runningOp)
			return m, publishArchiveCmd(path, m.market.desc, value)
		}
		if m.market.pkgIdx >= len(m.market.locals) {
			m.market.mode = marketSelectPkg
			return m, nil
		}
		pkg := m.market.locals[m.market.pkgIdx]
		m.market.mode = marketBrowseMode
		m.market.running = true
		m.market.progress = 0
		m.market.err = nil
		m.market.runningOp = "发布 " + pkg.Meta.ID
		m.setBusy(m.market.runningOp)
		return m, publishMarketCmd(pkg, m.market.desc, value)
	}
	return m, nil
}

// listMarketCmd 后台获取市场列表（tag 为标签筛选，空串=全部）
func listMarketCmd(tag string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetMarketConfig()
		if cfg == nil {
			return marketListMsg{err: fmt.Errorf("未配置 ltpx_market（lunar_config.json 缺少配置节）")}
		}
		items, err := ltpxmarket.ListPackages(*cfg, tag, 0)
		return marketListMsg{items: items, err: err}
	}
}

// scanLocalsCmd 后台扫描本地包（发布候选）
func scanLocalsCmd() tea.Cmd {
	return func() tea.Msg {
		cfg := GetMarketConfig()
		if cfg == nil {
			return marketLocalsMsg{err: fmt.Errorf("未配置 ltpx_market（lunar_config.json 缺少配置节）")}
		}
		items, err := ltpxmarket.ListLocalPackages(*cfg)
		return marketLocalsMsg{items: items, err: err}
	}
}

// scanArchiveCmd 后台读取已打包 .ltpx 文件的元数据（不解压）
func scanArchiveCmd(path string) tea.Cmd {
	return func() tea.Msg {
		pkg, err := ltpxmarket.ReadArchivePackage(path)
		return marketArchiveMsg{pkg: pkg, err: err}
	}
}

// installMarketCmd 后台安装市场包
func installMarketCmd(item ltpxmarket.MarketItem) tea.Cmd {
	return func() tea.Msg {
		cfg := GetMarketConfig()
		if cfg == nil {
			return marketDoneMsg{op: "安装", err: fmt.Errorf("未配置 ltpx_market（lunar_config.json 缺少配置节）")}
		}
		_, err := ltpxmarket.Install(*cfg, item)
		return marketDoneMsg{op: "安装 " + item.TagName, err: err}
	}
}

// publishMarketCmd 后台发布本地包（令牌仅注入副本，不污染全局配置）
func publishMarketCmd(pkg ltpxmarket.LocalPackage, desc, token string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetMarketConfig()
		if cfg == nil {
			return marketDoneMsg{op: "发布", err: fmt.Errorf("未配置 ltpx_market（lunar_config.json 缺少配置节）")}
		}
		cfgCopy := *cfg
		cfgCopy.GiteeToken = token
		err := ltpxmarket.Publish(cfgCopy, pkg, desc)
		return marketDoneMsg{op: "发布 " + pkg.Meta.ID, err: err}
	}
}

// publishArchiveCmd 后台发布已打包的 .ltpx 安装包（令牌仅注入副本，不污染全局配置）
func publishArchiveCmd(path, desc, token string) tea.Cmd {
	return func() tea.Msg {
		cfg := GetMarketConfig()
		if cfg == nil {
			return marketDoneMsg{op: "发布", err: fmt.Errorf("未配置 ltpx_market（lunar_config.json 缺少配置节）")}
		}
		cfgCopy := *cfg
		cfgCopy.GiteeToken = token
		err := ltpxmarket.PublishArchive(cfgCopy, path, desc)
		return marketDoneMsg{op: "发布 " + filepath.Base(path), err: err}
	}
}

// ==== 键位与上下文 ====

// marketContext 标题栏上下文：仓库 + 当前阶段 / 筛选条件
func marketContext(s marketState) string {
	repo := "未配置仓库"
	if cfg := GetMarketConfig(); cfg != nil && cfg.GiteeOwner != "" {
		repo = cfg.GiteeOwner + "/" + cfg.GiteeRepo
	}
	switch {
	case s.running:
		return repo + " · " + s.runningOp
	case s.mode == marketSelectPkg:
		return repo + " · 发布向导 1/3"
	case s.mode == marketInputArchive:
		return repo + " · 发布向导 · 指定安装包"
	case s.mode == marketInputDesc:
		return repo + " · 发布向导 2/3"
	case s.mode == marketInputToken:
		return repo + " · 发布向导 3/3"
	case s.mode == marketDetailMode:
		return repo + " · 包详情"
	}
	if s.tagHint != "" {
		repo += " · 标签 " + s.tagHint
	}
	if s.query != "" {
		repo += " · 搜索 " + s.query
	}
	// 与更新包共用同一仓库：标明本屏只消费 LTPX 通道，避免误以为仓库里没有更新包
	return repo + " · LTPX 通道"
}

// marketKeys 市场屏键位（随模式变化，底栏与帮助页同源）
func marketKeys(s marketState) []keyHint {
	if s.running {
		return []keyHint{kh("…", "任务进行中，完成前不可操作")}
	}
	switch s.mode {
	case marketInputTag, marketInputQuery, marketInputArchive, marketInputDesc, marketInputToken:
		return []keyHint{kh("Enter", "确认"), kh("Esc", "取消并返回")}
	case marketDetailMode:
		return []keyHint{
			kh("↑/↓ PgUp/PgDn", "滚动描述"),
			kh("Enter/i", "安装"),
			kh("Esc/←", "返回列表"),
		}
	case marketSelectPkg:
		return []keyHint{
			kh("↑/↓ PgUp/PgDn", "选择包"),
			kh("Enter", "下一步：填写描述"),
			kh("f", "上传已打包 .ltpx"),
			kh("Esc", "返回列表"),
		}
	}
	return []keyHint{
		kh("↑/↓", "选择"),
		kh("Enter", "详情"),
		kh("i", "安装"),
		kh("/", "搜索"),
		kh("t", "标签筛选"),
		kh("u", "发布本地包"),
		kh("r", "刷新"),
		kh("Esc", "返回菜单"),
	}
}

// ==== 渲染 ====

// viewMarket 市场屏渲染：统一框体 + 按模式切换内容区
func (m appModel) viewMarket() string {
	f := m.frameFor("LTPX 应用市场", marketContext(m.market), marketKeys(m.market))
	w, h := f.bodySize()

	var body []string
	switch {
	case m.market.running:
		body = marketRunningBody(m, w, h)
	case m.market.mode == marketInputTag || m.market.mode == marketInputQuery ||
		m.market.mode == marketInputArchive || m.market.mode == marketInputDesc ||
		m.market.mode == marketInputToken:
		body = m.marketInputBody(w)
	case m.market.mode == marketDetailMode:
		body = m.marketDetailBody(w, h)
	case m.market.mode == marketSelectPkg:
		body = m.marketSelectPkgBody(w, h)
	default:
		body = m.marketBrowseBody(w, h)
	}
	body = withErrStrip(body, m.market.err, w)

	// 底栏位置：列表态显示「第几项 / 共几项」，便于长时间滚动后仍知道身在何处
	switch m.market.mode {
	case marketBrowseMode, marketDetailMode:
		f.status = dimStyle.Render(posText(m.market.itemIdx, len(m.marketVisible()), "个包"))
	case marketSelectPkg:
		f.status = dimStyle.Render(posText(m.market.localIdx, len(m.market.locals), "个包"))
	}
	return f.render(body)
}

// marketRunningBody 任务进行中：spinner + 任务名 + 进度条
func marketRunningBody(m appModel, w, h int) []string {
	lines := []string{
		"",
		"  " + selectStyle.Render(m.spinner.View()) + " " + valueStyle.Render(m.market.runningOp+"…"),
		"",
	}
	if m.market.progress > 0 {
		lines = append(lines, "  "+progressBar(m.market.progress, minInt(w-8, 60)))
	} else {
		lines = append(lines, "  "+dimStyle.Render("准备中：正在建立通道并读取远端数据"))
	}
	lines = append(lines, "", "  "+dimStyle.Render("详细过程见「原始日志」；任务可在后台继续完成"))
	return fitLines(lines, h)
}

// marketBrowseBody 市场列表态：宽屏两栏（列表 + 预览），窄屏单栏
func (m appModel) marketBrowseBody(w, h int) []string {
	items := m.marketVisible()
	if len(items) == 0 {
		msg := "市场暂无包（按 [u] 发布第一个 LTPX 工具包）"
		switch {
		case m.market.query != "":
			msg = "没有匹配「" + m.market.query + "」的包（按 [/] 修改或清空搜索）"
		case m.market.tagHint != "":
			msg = "没有带标签 " + m.market.tagHint + " 的包（按 [t] 清空筛选重试）"
		}
		return centerBlock([]string{"", "", dimStyle.Render(msg)}, w)
	}

	idx := clampInt(m.market.itemIdx, 0, len(items)-1)
	twoPane := w >= 88
	listW, detailW := w, 0
	if twoPane {
		listW = clampInt(w*52/100, 40, 60)
		detailW = w - listW - 4 // 竖分隔 3 列 + 右栏尾部留白 1 列
	}
	rowW := maxInt(8, listW-listGutter)

	blocks := make([][]string, len(items))
	for i, it := range items {
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		right := marketOrDash(it.TagName)
		if it.AssetSize > 0 {
			right += "  " + humanSize(it.AssetSize)
		}
		title := labelStyle.Render("◆ " + marketOrDash(it.Title))
		right = dimStyle.Render(right)
		if sel {
			title = selectStyle.Render("◆ " + marketOrDash(it.Title))
			right = keyStyle.Render(right)
		}
		blocks[i] = []string{listRow(rowW, sel, cursor, title, right)}
	}

	lines, _ := m.market.browsePl.layoutView(blocks, idx, listW, h)
	body := lines
	if twoPane {
		preview := marketPreview(items[idx], detailW, h)
		body = make([]string, h)
		for i := 0; i < h; i++ {
			body[i] = lines[i] + ruleStyle.Render(" │ ") + preview[i] + " "
		}
	}
	return body
}

// marketPreview 右侧预览栏：包元信息 + 描述（超出部分截断，提示进入详情看全文）
func marketPreview(it ltpxmarket.MarketItem, w, h int) []string {
	if w <= 0 {
		return blankLines(h, w)
	}
	lines := []string{
		selectStyle.Render("◆ " + marketOrDash(it.Title)),
		dimStyle.Render(marketOrDash(it.TagName)),
		"",
		kvRow("标签", marketOrDash(strings.Join(it.Tags, " · ")), 6),
		kvRow("附件", humanSize(it.AssetSize), 6),
		"",
		sectionRule("描述", w),
	}
	desc := strings.TrimSpace(it.Description)
	if desc == "" {
		desc = "（该包未提供描述）"
	}
	room := maxInt(1, h-len(lines)-2)
	for _, l := range wrapText(desc, maxInt(8, w-2), room) {
		lines = append(lines, "  "+labelStyle.Render(l))
	}
	lines = append(lines, "", dimStyle.Render("  Enter 查看详情 / 安装"))
	return fitLines(lines, h)
}

// marketDetailBody 详情态：元信息头 + 可滚动描述全文
func (m appModel) marketDetailBody(w, h int) []string {
	items := m.marketVisible()
	if len(items) == 0 {
		return blankLines(h, w)
	}
	idx := clampInt(m.market.itemIdx, 0, len(items)-1)
	it := items[idx]

	head := []string{
		selectStyle.Render("◆ "+marketOrDash(it.Title)) + "  " + dimStyle.Render(marketOrDash(it.TagName)),
		dimStyle.Render("  标签: "+marketOrDash(strings.Join(it.Tags, " · "))) +
			dimStyle.Render("    附件: "+humanSize(it.AssetSize)),
		"",
	}
	room := maxInt(3, h-len(head)-2)
	desc := strings.TrimSpace(it.Description)
	if desc == "" {
		desc = "（该包未提供描述）"
	}
	lines, info := m.market.detailPl.win(desc, maxInt(8, w-2), room)
	body := append([]string{}, head...)
	for _, l := range lines {
		body = append(body, "  "+labelStyle.Render(l))
	}
	if info.Total > info.View {
		body = append(body, dimStyle.Render(fmt.Sprintf("  ↕ %d/%d 行（↑/↓ PgUp/PgDn 滚动）", info.Top+1, info.Total)))
	}
	return fitLines(body, h)
}

// marketSelectPkgBody 发布第一步：本地包选择。
//
// 列表经定高窗口渲染：条目再多也只占用内容区高度，
// 方向键移动选中项时窗口随之滚动——这就是「列表顶出终端且不滚动」的修复点。
func (m appModel) marketSelectPkgBody(w, h int) []string {
	head := []string{
		stepsBar(marketPublishSteps, 0, w),
		sectionRule("① 选择本地包", w),
	}
	if m.market.archivePath != "" {
		head = append(head, okStyle.Render("  已指定安装包：")+valueStyle.Render(m.market.archivePkg.DirName)+
			dimStyle.Render("   （改回目录发布请在列表中重新选择）"))
	} else {
		head = append(head, dimStyle.Render("  回车进入描述填写；已有琉璃导出的 .ltpx 安装包时按 [f] 直接上传"))
	}
	head = append(head, "")

	if len(m.market.locals) == 0 {
		body := append([]string{}, head...)
		body = append(body,
			dimStyle.Render("  包目录中未发现 LTPX 包（形如 */metadata.json）"),
			"",
			dimStyle.Render("  可用 [f] 上传已打包的 .ltpx 安装包"))
		return fitLines(body, h)
	}

	idx := clampInt(m.market.localIdx, 0, len(m.market.locals)-1)
	rowW := maxInt(8, w-listGutter)
	blocks := make([][]string, len(m.market.locals))
	for i := range m.market.locals {
		pkg := m.market.locals[i]
		sel := i == idx
		cursor := "  "
		if sel {
			cursor = cursorStyle.Render("▸ ")
		}
		sub := "    " + dimStyle.Render("id: "+marketOrDash(pkg.Meta.ID)+" · 标签: "+marketOrDash(strings.Join(pkg.Meta.Tags, " · ")))
		title := labelStyle.Render("◆ " + pkg.DirName)
		size := dimStyle.Render(humanSize(pkg.Size))
		if sel {
			sub = "    " + selDimStyle.Render("id: "+marketOrDash(pkg.Meta.ID)+" · 标签: "+marketOrDash(strings.Join(pkg.Meta.Tags, " · ")))
			title = selectStyle.Render("◆ " + pkg.DirName)
			size = keyStyle.Render(humanSize(pkg.Size))
		}
		blocks[i] = []string{
			listRow(rowW, sel, cursor, title, size),
			truncTo(sub, rowW),
		}
	}

	lines, _ := m.market.pkgPl.layoutView(blocks, idx, w, maxInt(1, h-len(head)))

	body := append([]string{}, head...)
	body = append(body, lines...)
	return fitLines(body, h)
}

// marketInputBody 输入态：步骤条 + 提问 + 输入框 + 说明
func (m appModel) marketInputBody(w int) []string {
	var body []string
	var prompt, hint string

	switch m.market.mode {
	case marketInputTag:
		prompt = "输入标签筛选（留空回车显示全部）："
		hint = "标签筛选会走远端接口重新拉取列表，例如 Node-LTP"
	case marketInputQuery:
		prompt = "输入关键词搜索（本地过滤，不重新拉取）："
		hint = "匹配包名 / 标题 / 标签 / 描述；留空回车即清除搜索"
	case marketInputArchive:
		body = append(body, stepsBar(marketPublishSteps, 0, w), "")
		prompt = "输入已打包的 .ltpx 文件路径（琉璃导出的安装包，.zip 亦可）："
		hint = "从包内读取 metadata.json 识别包信息（不解压）；留空回车取消"
	case marketInputDesc:
		body = append(body, stepsBar(marketPublishSteps, 1, w), "")
		prompt = "发布 " + m.publishSourceName() + " —— 输入描述文本（#标签 会被自动识别）："
		hint = "留空回车则使用包 metadata.json 中的描述"
	case marketInputToken:
		body = append(body, stepsBar(marketPublishSteps, 2, w), "")
		prompt = "输入 Gitee 私人令牌（发布需 Release 读写权限，留空回车取消）："
		hint = "令牌仅用于本次发布，不写入配置文件；同 id 包发布会覆盖旧发布"
	}

	body = append(body, "", "  "+labelStyle.Render(prompt), "")
	boxW := minInt(w, 78)
	body = append(body, formBox([]string{"", "  " + m.market.input.View(), ""}, boxW)...)
	body = append(body, "", "  "+dimStyle.Render(hint))

	if m.market.mode == marketInputDesc || m.market.mode == marketInputToken {
		body = append(body, "", "  "+dimStyle.Render("发布源: ")+valueStyle.Render(m.publishSourceName()))
		if d := strings.TrimSpace(m.market.desc); d != "" && m.market.mode == marketInputToken {
			body = append(body, "  "+dimStyle.Render("描述:   ")+labelStyle.Render(truncTo(firstLine(d), maxInt(8, boxW-12))))
		}
	}
	return body
}

// publishSourceName 发布源展示名：已打包文件取包标题（括注文件名），目录取目录名
func (m appModel) publishSourceName() string {
	if m.market.archivePath != "" {
		name := marketOrDash(m.market.archivePkg.Meta.Title)
		return fmt.Sprintf("%s（%s）", name, m.market.archivePkg.DirName)
	}
	if m.market.pkgIdx < len(m.market.locals) {
		return m.market.locals[m.market.pkgIdx].DirName
	}
	return "-"
}

// marketOrDash 空串展示为破折号
func marketOrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
