package TuiSystem

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"LunarSubsystem/TuiSystem/archive"
	"LunarSubsystem/TuiSystem/ltpxmarket"
	"LunarSubsystem/TuiSystem/updatepack"
)

// TestMain 强制彩色渲染：测试环境没有 TTY，默认渲染器会剥掉全部样式，
// 导致「按显示宽度排版」的断言退化成纯文本比较而失去意义。
// 打开真彩色后，宽度断言才真正覆盖 ANSI 序列与宽字符混合的情形。
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// termSizes 覆盖常用终端尺寸（含极窄极矮的边界情形）
var termSizes = []struct {
	name string
	w, h int
}{
	{"宽屏", 140, 45},
	{"常规", 100, 30},
	{"紧凑", 80, 24},
	{"窄屏", 60, 18},
	{"临界", 46, 11},
	{"过小", 30, 8},
}

// assertFits 断言整屏渲染不越界：行数不超过终端高度、每行显示宽度不超过终端宽度。
// 这是「列表超出终端高度」「长行把框线挤破」两类排版缺陷的通用守卫。
func assertFits(t *testing.T, name string, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) > h {
		t.Fatalf("%s: 渲染 %d 行，超出终端高度 %d", name, len(lines), h)
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got > w {
			t.Fatalf("%s: 第 %d 行显示宽度 %d 超出终端宽度 %d\n  内容: %q",
				name, i+1, got, w, ansi.Strip(l))
		}
	}
}

// assertExactFrame 断言正常尺寸下框体行数恰好等于终端高度
// （框体必须严丝合缝地占满屏幕，否则会出现撕裂或残留）
func assertExactFrame(t *testing.T, name, view string, w, h int) {
	t.Helper()
	if w < frameMinWidth || h < frameMinHeight {
		return
	}
	if got := len(strings.Split(view, "\n")); got != h {
		t.Fatalf("%s: 框体渲染 %d 行，应恰好 %d 行", name, got, h)
	}
}

// ==== 测试数据构造 ====

// makeMarketItems 构造 n 个市场包（标题/标签/描述均带序号，便于断言窗口内容）
func makeMarketItems(n int) []ltpxmarket.MarketItem {
	items := make([]ltpxmarket.MarketItem, n)
	for i := range items {
		id := fmt.Sprintf("pkg-%02d", i+1)
		items[i] = ltpxmarket.MarketItem{
			TagName:     id,
			Title:       "工具包 " + id,
			Description: "这是第 " + id + " 个工具包的描述文本，用于验证长列表的窗口滚动与中文折行排版是否正确。",
			Tags:        []string{"Node-LTP", "实用工具"},
			AssetURL:    "https://example.com/" + id + ".ltpx",
			AssetSize:   int64(1024 * 1024 * (i + 1)),
		}
	}
	return items
}

// makeLocalPackages 构造 n 个本地包（发布候选）
func makeLocalPackages(n int) []ltpxmarket.LocalPackage {
	out := make([]ltpxmarket.LocalPackage, n)
	for i := range out {
		id := fmt.Sprintf("pkg-%02d", i+1)
		out[i] = ltpxmarket.LocalPackage{
			DirName: id,
			Dir:     "local_data/package/" + id,
			Size:    int64(1024*1024*(i+1)) + 512,
			Meta: ltpxmarket.PackageMeta{
				ID:    id,
				Title: "工具包 " + id,
				Tags:  []string{"Node-LTP", "图像工坊"},
			},
		}
	}
	return out
}

// makeVersions 构造 n 个已发布版本
func makeVersions(n int) []updatepack.Version {
	out := make([]updatepack.Version, n)
	for i := range out {
		out[i] = updatepack.Version{
			TagName: fmt.Sprintf("v2026.%02d.14", i+1),
			Name:    fmt.Sprintf("第 %d 个版本", i+1),
			Body:    "本次更新包含若干修复与改进，正文可能较长因而需要折行展示。",
		}
	}
	return out
}

// makeConfigPaths 构造 n 个候选打包配置路径
func makeConfigPaths(n int) []string {
	out := make([]string, n)
	for i := range out {
		if i == 0 {
			out[i] = archive.DefaultConfigPath
			continue
		}
		out[i] = fmt.Sprintf("data/archive_config_%02d.json", i+1)
	}
	return out
}

// seedLogs 向指定来源灌入 n 条日志（含超长行，用于验证软折行）
func seedLogs(source LogSource, n int) {
	ClearLogs(source)
	for i := 0; i < n; i++ {
		pushLog(LogLine{
			Time:   time.Now(),
			Source: "log",
			Level:  []string{"INFO", "WARN", "ERROR"}[i%3],
			Module: "Test",
			Text: fmt.Sprintf("第 %03d 条日志：这是一条用于验证软折行与滚动窗口的中文长消息，"+
				"重复内容以便超过终端宽度并触发折行处理。", i),
		}, source)
	}
}

// seedAgentTurns 灌入若干轮智能体实况（全局状态，供实况屏渲染）
func seedAgentTurns(n int) {
	for i := 0; i < n; i++ {
		role := []string{"对话者", "绘制者", "演奏者", "记忆者"}[i%4]
		StreamBegin(role)
		StreamDelta(role, KindThink, "先分析一下用户意图，再决定调用哪个工具。这段思考文本会以暗色显示。\n")
		StreamTool(role, "search_weather", `{"city":"上海"}`)
		StreamDelta(role, KindContent, "已完成第 "+fmt.Sprint(i+1)+" 轮处理，输出内容用于验证实况屏的窗口滚动与折行排版是否正确。\n")
		StreamEnd(role)
	}
}

// ==== 用例矩阵 ====

// screenCase 一个渲染用例（某屏的某个子模式）
type screenCase struct {
	name  string
	build func(w, h int) appModel
}

// richModel 构建带充足内容的模型骨架
func richModel(id screenID, w, h int) appModel {
	m := newAppModel()
	m.width, m.height = w, h
	m.screen = id
	return m
}

// screenCases 全屏 × 关键子模式的渲染用例
func screenCases() []screenCase {
	return []screenCase{
		{"主菜单", func(w, h int) appModel { return richModel(screenMenu, w, h) }},
		{"键位帮助", func(w, h int) appModel {
			m := richModel(screenMenu, w, h)
			m.help = true
			return m
		}},
		{"智能体实况", func(w, h int) appModel { return richModel(screenAgent, w, h) }},
		{"运行状态", func(w, h int) appModel {
			m := richModel(screenStats, w, h)
			m.stats.snap = GetSnapshot()
			return m
		}},
		{"原始日志", func(w, h int) appModel {
			seedLogs(SourceRaw, 300)
			return richModel(screenRaw, w, h)
		}},
		{"月华JS日志", func(w, h int) appModel {
			seedLogs(SourceLunarJS, 120)
			return richModel(screenLunarJS, w, h)
		}},
		{"插件JS日志", func(w, h int) appModel {
			seedLogs(SourceLTP9JS, 60)
			return richModel(screenLTP9, w, h)
		}},
		{"绘图引擎日志", func(w, h int) appModel {
			seedLogs(SourceDraw, 80)
			m := richModel(screenDraw, w, h)
			m.drawView.reset()
			return m
		}},
		{"Go警告", func(w, h int) appModel {
			seedLogs(SourceGoWarn, 40)
			return richModel(screenGoWarn, w, h)
		}},
		{"前端管理", func(w, h int) appModel { return richModel(screenFrontend, w, h) }},
		{"端口释放", func(w, h int) appModel { return richModel(screenPorts, w, h) }},
		{"代理解析态", func(w, h int) appModel {
			m := richModel(screenProxy, w, h)
			m.proxy.activated = true
			m.proxy.ips = []string{"192.168.1.7", "10.0.0.3"}
			m.applyProxyFocus(0)
			return m
		}},
		{"代理运行态", func(w, h int) appModel {
			m := richModel(screenProxy, w, h)
			m.proxy.running = true
			m.proxy.info.IP = "192.168.1.7"
			m.proxy.info.BackendPort = 36789
			m.proxy.info.ProxyPort = 36790
			m.proxy.qrText = generateQRText("https://192.168.1.7:36790")
			return m
		}},
		{"打包配置列表", func(w, h int) appModel {
			m := richModel(screenArchive, w, h)
			m.archive.configs = makeConfigPaths(24)
			return m
		}},
		{"打包配置空态", func(w, h int) appModel {
			m := richModel(screenArchive, w, h)
			m.archive.configs = nil
			return m
		}},
		{"打包路径输入", func(w, h int) appModel {
			m := richModel(screenArchive, w, h)
			m.archive.configs = makeConfigPaths(3)
			m.archive.inputMode = true
			m.archive.input.Focus()
			return m
		}},
		{"打包确认态", func(w, h int) appModel {
			m := richModel(screenArchive, w, h)
			m.archive.loaded = &archive.Config{
				Output: "build/lunar.7z", Size: 100, Level: 5,
				Import:  []string{"subsystem", "local_data", "docs", "README.md"},
				Exclude: []string{"*.log", "node_modules", "__pycache__"},
			}
			m.archive.loadedPath = archive.DefaultConfigPath
			return m
		}},
		{"更新包菜单", func(w, h int) appModel { return richModel(screenUpdate, w, h) }},
		{"更新包版本列表", func(w, h int) appModel {
			m := richModel(screenUpdate, w, h)
			m.update.mode = updateVersions
			m.update.versions = makeVersions(30)
			return m
		}},
		{"更新包输入", func(w, h int) appModel {
			m := richModel(screenUpdate, w, h)
			m.update.mode = updateInputToken
			m.update.input.Focus()
			return m
		}},
		{"更新包交接", func(w, h int) appModel {
			m := richModel(screenUpdate, w, h)
			m.update.mode = updateHandover
			m.update.handoverLeft = handoverSeconds
			// 交接画面读包级交接说明（控制台退出后 Run 还要用它打印终端提示）
			pendingHandover = &handoverNote{
				PackFile: "lunar_updatepack_v2026.10.15-2128.7z.001",
				Program:  `D:\Lunar_Astral_Agents\Lunar_Astral_Agents.exe`,
				Root:     `D:\Lunar_Astral_Agents`,
				Files:    []string{"Lunar_Astral_Agents.exe", "Lunar_Update_Applier.exe"},
				LogPath:  `D:\Lunar_Astral_Agents\local_data\logs\update_apply.log`,
				Applier:  `D:\Lunar_Astral_Agents\Lunar_Update_Applier.exe`,
			}
			return m
		}},
		{"市场列表", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.loaded = true
			m.market.items = makeMarketItems(40)
			return m
		}},
		{"市场空态", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.loaded = true
			return m
		}},
		{"市场详情", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.loaded = true
			m.market.items = makeMarketItems(6)
			m.market.mode = marketDetailMode
			return m
		}},
		{"市场发布选包", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.loaded = true
			m.market.items = makeMarketItems(4)
			m.market.locals = makeLocalPackages(40)
			m.market.mode = marketSelectPkg
			return m
		}},
		{"市场发布描述", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.locals = makeLocalPackages(3)
			m.market.mode = marketInputDesc
			m.market.input.Focus()
			return m
		}},
		{"市场任务中", func(w, h int) appModel {
			m := richModel(screenMarket, w, h)
			m.market.running = true
			m.market.runningOp = "发布 pkg-01"
			m.market.progress = 42
			return m
		}},
	}
}

// TestAllScreensFitTerminal 全屏 × 多尺寸的边界守卫：
// 任何屏幕在任何尺寸下都不得把内容顶出终端，正常尺寸下框体须恰好占满屏幕。
func TestAllScreensFitTerminal(t *testing.T) {
	seedAgentTurns(12) // 实况屏内容来自全局轮次缓冲
	for _, tc := range screenCases() {
		for _, size := range termSizes {
			m := tc.build(size.w, size.h)
			view := m.View()
			assertFits(t, fmt.Sprintf("%s@%s(%d×%d)", tc.name, size.name, size.w, size.h), view, size.w, size.h)
			assertExactFrame(t, fmt.Sprintf("%s@%s(%d×%d)", tc.name, size.name, size.w, size.h), view, size.w, size.h)
		}
	}
}

// TestEveryScreenReachable 保证用例矩阵覆盖全部屏幕编号（新增屏幕时必须补用例）
func TestEveryScreenReachable(t *testing.T) {
	covered := map[screenID]bool{}
	for _, c := range screenCases() {
		covered[c.build(100, 30).screen] = true
	}
	for _, id := range []screenID{
		screenMenu, screenAgent, screenStats, screenRaw, screenLunarJS,
		screenLTP9, screenDraw, screenGoWarn, screenFrontend, screenProxy,
		screenArchive, screenUpdate, screenPorts, screenMarket,
	} {
		if !covered[id] {
			t.Fatalf("屏幕 %d 未被渲染用例覆盖", id)
		}
	}
}

// TestMarketPublishListScrollsWithinTerminal 发布选包列表的缺陷回归：
// 包数量远超终端高度时，① 列表不得顶出终端；② 移动选中项时窗口必须随之滚动。
func TestMarketPublishListScrollsWithinTerminal(t *testing.T) {
	const (
		w, h = 80, 24
		n    = 40
	)
	m := newAppModel()
	m.width, m.height = w, h
	m.screen = screenMarket
	m.market.loaded = true
	m.market.items = makeMarketItems(4)
	m.market.locals = makeLocalPackages(n)
	m.market.mode = marketSelectPkg

	view := m.View()
	assertFits(t, "发布选包·首屏", view, w, h)
	if !strings.Contains(view, "pkg-01") {
		t.Fatalf("首屏应显示第一个包\n%s", ansi.Strip(view))
	}
	if strings.Contains(view, "pkg-40") {
		t.Fatalf("首屏不应显示列表末尾的包（说明未做窗口化）\n%s", ansi.Strip(view))
	}

	// 逐条下移到底：每一步都必须保持在终端内，且选中项可见
	for i := 0; i < n-1; i++ {
		nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = nm.(appModel)
		if m.market.localIdx != i+1 {
			t.Fatalf("第 %d 次下移后选中项应为 %d，实际 %d", i+1, i+1, m.market.localIdx)
		}
		step := m.View()
		assertFits(t, fmt.Sprintf("发布选包·下移到 %d", i+1), step, w, h)
		want := fmt.Sprintf("pkg-%02d", i+1)
		if !strings.Contains(step, want) {
			t.Fatalf("选中项 %s 未出现在窗口内（滚动未跟随选中项）\n%s", want, ansi.Strip(step))
		}
	}

	end := m.View()
	if !strings.Contains(end, "pkg-40") {
		t.Fatalf("滚到底后应显示最后一个包\n%s", ansi.Strip(end))
	}
	if strings.Contains(end, "pkg-01") {
		t.Fatalf("滚到底后不应再显示第一个包（窗口未滚动）\n%s", ansi.Strip(end))
	}
	if !strings.Contains(end, fmt.Sprintf("%d/%d", n, n)) {
		t.Fatalf("底栏应显示位置 %d/%d", n, n)
	}
}

// TestLogScreenScrollsAndPausesFollow 日志屏：默认跟随底部，向上滚动即暂停跟随，
// 按 End 恢复跟随；滚动范围受内容行数约束。
func TestLogScreenScrollsAndPausesFollow(t *testing.T) {
	seedLogs(SourceRaw, 200)
	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenRaw

	if view := m.View(); !strings.Contains(view, "跟随中") {
		t.Fatalf("日志屏默认应处于跟随状态\n%s", ansi.Strip(view))
	}

	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = nm.(appModel)
	if m.rawView.follow {
		t.Fatalf("向上翻页后应暂停跟随")
	}
	if m.rawView.offset <= 0 {
		t.Fatalf("向上翻页后应离开底部，实际 offset=%d", m.rawView.offset)
	}
	assertFits(t, "日志·暂停跟随", m.View(), m.width, m.height)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = nm.(appModel)
	if !m.rawView.follow {
		t.Fatalf("按 End 应恢复跟随")
	}
}

// TestFrameGuaranteesBounds 框体本身的行数/列宽约束（含超长内容与超量内容）
func TestFrameGuaranteesBounds(t *testing.T) {
	f := frame{width: 60, height: 12, brand: "测试", title: "标题", context: "上下文"}
	body := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		body = append(body, strings.Repeat("超长内容", 20))
	}
	view := f.render(body)
	assertFits(t, "框体超量内容", view, 60, 12)
	assertExactFrame(t, "框体超量内容", view, 60, 12)

	// 极窄尺寸：不得 panic，且不得越界
	tiny := frame{width: 10, height: 3, brand: "测试", title: "标题"}
	assertFits(t, "框体极小", tiny.render([]string{"内容"}), 10, 3)
}

// TestFrameRuleCounterAlignment 标题栏与键位栏在空间不足时仍恰好等于 renderWidth。
//
// 注意口径：框线恒为 renderWidth = 终端宽度 - 1（末列留空，见 frame.renderWidth 的说明：
// conhost 上写满整行的行会在换行时丢掉最后一个字符——实测框线右角就是这么消失的）。
// 因此这里断言「恰好 renderWidth」且「不超过终端宽度」。
func TestFrameRuleCounterAlignment(t *testing.T) {
	for _, w := range []int{200, 120, 80, 60, 46, 30, 20, 10, 4, 2} {
		f := frame{
			width: w, height: 12, brand: "星月智能 · 控制台", title: "LTPX 应用市场",
			context: "owner/repo · 发布向导 1/3",
			extra:   "1h02m03s · 12:34:56",
			keys: []keyHint{
				kh("↑/↓ PgUp/PgDn", "选择包（列表自动滚动）"),
				kh("Enter", "下一步：填写描述"),
				kh("f", "改为上传已打包 .ltpx"),
				kh("Esc", "返回列表"),
			},
			status: "40/40 个包",
		}
		want := f.renderWidth()
		for name, rule := range map[string]string{"标题栏": f.topRule(), "键位栏": f.bottomRule()} {
			if got := dispWidth(rule); got != want {
				t.Fatalf("%s 宽度 %d ≠ 排版宽度 %d（终端 %d，%q）", name, got, want, w, ansi.Strip(rule))
			}
			if dispWidth(rule) > w {
				t.Fatalf("%s 宽度 %d 超过终端宽度 %d", name, dispWidth(rule), w)
			}
		}
	}
}

// keyWalk 交互遍历用的按键集合（覆盖导航、翻页、开关、快捷键与自由输入）
func keyWalk() []tea.KeyMsg {
	runes := []string{
		"?", "f", "c", "r", "u", "i", "t", "/", "p", "q", "e",
		"1", "2", "3", "4", "5", "0", "a", "b", "m", "z",
		"k", "j", "g", "G", "N",
	}
	out := []tea.KeyMsg{
		{Type: tea.KeyEsc},
		{Type: tea.KeyUp}, {Type: tea.KeyDown}, {Type: tea.KeyLeft}, {Type: tea.KeyRight},
		{Type: tea.KeyEnter}, {Type: tea.KeyTab},
		{Type: tea.KeyPgUp}, {Type: tea.KeyPgDown}, {Type: tea.KeyHome}, {Type: tea.KeyEnd},
		{Type: tea.KeyBackspace},
	}
	for _, r := range runes {
		out = append(out, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
	}
	return out
}

// TestKeyWalkNoPanic 交互遍历：向各屏（含各子模式）依次投递常用按键与窗口尺寸变化，
// 断言全程不 panic、渲染始终不越界。
//
// 刻意**丢弃命令**：tea.Cmd 是惰性的，不执行就不会真的发起网络拉取、打包或端口释放，
// 因此本测试没有外部副作用，却覆盖了「按键 → 状态迁移 → 渲染」的完整链路。
func TestKeyWalkNoPanic(t *testing.T) {
	seedAgentTurns(4)
	seedLogs(SourceRaw, 60)
	keys := keyWalk()

	for _, tc := range screenCases() {
		m := tc.build(100, 30)
		for i, k := range keys {
			nm, _ := m.Update(k)
			m = nm.(appModel)

			view := m.View()
			assertFits(t, fmt.Sprintf("%s·第 %d 键(%s)", tc.name, i+1, describeKey(k)), view, m.width, m.height)
			assertExactFrame(t, fmt.Sprintf("%s·第 %d 键(%s)", tc.name, i+1, describeKey(k)), view, m.width, m.height)

			// 中途改变窗口尺寸，验证缩放后仍不越界
			if i%7 == 3 {
				nm, _ = m.Update(tea.WindowSizeMsg{Width: 72, Height: 20})
				m = nm.(appModel)
				assertFits(t, fmt.Sprintf("%s·缩放后", tc.name), m.View(), 72, 20)
				nm, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
				m = nm.(appModel)
			}
		}
	}
}

// describeKey 按键的可读描述（失败信息里便于定位）
func describeKey(k tea.KeyMsg) string {
	if len(k.Runes) > 0 {
		return string(k.Runes)
	}
	return k.Type.String()
}
