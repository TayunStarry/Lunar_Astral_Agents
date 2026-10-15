package TuiSystem

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/TuiSystem/ltpxmarket"
)

// driveKeys 依次向模型投递按键并同步执行返回的命令、回填其消息。
// 仅执行数据命令（u 扫描本地包 / enter 提交）：其余按键（逐字符输入、f 等）
// 返回的是 textinput 光标闪烁命令，其实现会等待 BlinkSpeed（约 530ms）才返回，
// 逐键执行会让流程测试空转数十秒；且这些命令无状态副作用，跳过不影响断言。
func driveKeys(t *testing.T, m appModel, keys ...string) appModel {
	t.Helper()
	for _, k := range keys {
		var runes []rune
		isSpecial := false
		switch k {
		case "enter":
			runes, isSpecial = nil, true
		case "esc", "up", "down":
			runes, isSpecial = nil, true
		default:
			runes = []rune(k)
		}

		var nm tea.Model
		var cmd tea.Cmd
		if isSpecial {
			keyType := tea.KeyEnter
			switch k {
			case "esc":
				keyType = tea.KeyEscape
			case "up":
				keyType = tea.KeyUp
			case "down":
				keyType = tea.KeyDown
			}
			nm, cmd = m.Update(tea.KeyMsg{Type: keyType})
		} else {
			nm, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: runes})
		}
		m = nm.(appModel)
		// 数据命令同步执行并回填（配置指向不存在仓库时离线返回错误消息，无网络副作用）
		if cmd != nil && (k == "u" || k == "enter") {
			if msg := cmd(); msg != nil {
				nm, _ = m.Update(msg)
				m = nm.(appModel)
			}
		}
	}
	return m
}

// TestMenuMarketHotkey 菜单按 m 直达应用市场屏（进入时自动拉列表，未配置时离线报错）
func TestMenuMarketHotkey(t *testing.T) {
	SetMarketConfig(nil)
	defer SetMarketConfig(nil)

	m := newAppModel()
	m.width, m.height = 100, 30
	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	m = nm.(appModel)
	if m.screen != screenMarket {
		t.Fatalf("按 m 应进入应用市场屏，实际 screen=%d", m.screen)
	}

	// 进入时自动拉取列表：未配置 → 拉取命令离线返回错误消息
	if cmd == nil {
		t.Fatalf("列表未加载时应返回拉取命令")
	}
	if msg := cmd(); msg != nil {
		nm, _ = m.Update(msg)
		m = nm.(appModel)
	}
	if m.market.err == nil {
		t.Fatalf("未配置 ltpx_market 时应呈现配置错误")
	}
	if m.market.loaded {
		t.Fatalf("拉取失败不应置 loaded")
	}
	if got := m.viewMarket(); got == "" {
		t.Fatalf("市场屏渲染不应为空")
	}
}

// TestMarketTagInputAcceptsRunes 标签筛选输入态可接收字符，回车后作为筛选条件拉取
func TestMarketTagInputAcceptsRunes(t *testing.T) {
	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenMarket
	m.market.loaded = true // 模拟已加载列表，绕过进入时的自动拉取
	SetMarketConfig(nil)   // 未配置：提交后的拉取命令离线返回错误消息，无网络副作用
	defer SetMarketConfig(nil)

	m = driveKeys(t, m, "t")
	if m.market.mode != marketInputTag {
		t.Fatalf("按 t 应进入标签输入态，实际 mode=%d", m.market.mode)
	}
	if !m.market.input.Focused() {
		t.Fatalf("标签输入框应已 Focus（未聚焦时 textinput 静默丢键）")
	}
	m = driveKeys(t, m, "N", "o", "d", "e", "-", "L", "T", "P")
	if got := m.market.input.Value(); got != "Node-LTP" {
		t.Fatalf("标签输入框未累积字符: %q", got)
	}

	// 回车 → 提交筛选并返回列表态；筛选命令离线执行（未配置仓库属主不存在仅报错，无写入）
	m = driveKeys(t, m, "enter")
	if m.market.tagHint != "Node-LTP" {
		t.Fatalf("tagHint 应为 Node-LTP，实际 %q", m.market.tagHint)
	}
	if m.market.mode != marketBrowseMode {
		t.Fatalf("提交后应回列表态，实际 mode=%d", m.market.mode)
	}
	if m.market.input.Focused() {
		t.Fatalf("提交后输入框应已 Blur")
	}
}

// TestMarketBrowseNavigationAndDetail 列表态方向键导航与详情进入/返回
func TestMarketBrowseNavigationAndDetail(t *testing.T) {
	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenMarket
	m.market.loaded = true
	m.market.items = []ltpxmarket.MarketItem{
		{TagName: "pkg.a", Title: "包A", Description: "第一个包", Tags: []string{"T1"}, AssetURL: "https://example.com/a.zip"},
		{TagName: "pkg.b", Title: "包B", Description: "第二个包", Tags: []string{"T2"}},
	}

	m = driveKeys(t, m, "down")
	if m.market.itemIdx != 1 {
		t.Fatalf("down 后应选中第 2 项，实际 %d", m.market.itemIdx)
	}
	m = driveKeys(t, m, "up", "up")
	if m.market.itemIdx != 1 {
		t.Fatalf("两次 up 应循环回第 2 项，实际 %d", m.market.itemIdx)
	}

	m = driveKeys(t, m, "enter")
	if m.market.mode != marketDetailMode {
		t.Fatalf("enter 应进入详情态")
	}
	if got := m.viewMarket(); got == "" {
		t.Fatalf("详情渲染不应为空")
	}

	// Esc 回列表
	m = driveKeys(t, m, "esc")
	if m.market.mode != marketBrowseMode {
		t.Fatalf("esc 应回列表态")
	}
}

// TestMarketPublishFlowInputStates 发布向导三步输入态与 Focus/Blur 状态迁移
func TestMarketPublishFlowInputStates(t *testing.T) {
	// 注入测试配置：包目录指向临时目录（含一个假包），仓库指向不存在的属主（避免真实发布）
	pkgRoot := t.TempDir()
	pkgDir := filepath.Join(pkgRoot, "fake.pkg")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatal(err)
	}
	meta := `{"id":"fake.pkg","title":"假包","description":"测试用","tags":["T9"]}`
	if err := os.WriteFile(filepath.Join(pkgDir, "metadata.json"), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}
	SetMarketConfig(&ltpxmarket.Config{GiteeOwner: "test-none", GiteeRepo: "none", PackageDir: pkgRoot})
	defer SetMarketConfig(nil)

	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenMarket
	m.market.loaded = true

	// u → 扫描本地包（命令同步执行）→ 包选择态
	m = driveKeys(t, m, "u")
	if m.market.mode != marketSelectPkg {
		t.Fatalf("u 后应进入包选择态，实际 mode=%d（err=%v）", m.market.mode, m.market.err)
	}
	if len(m.market.locals) != 1 || m.market.locals[0].Meta.ID != "fake.pkg" {
		t.Fatalf("应扫描到 fake.pkg，实际 %+v", m.market.locals)
	}

	// enter → 描述输入态（聚焦）；输入带 #标签 的描述
	m = driveKeys(t, m, "enter")
	if m.market.mode != marketInputDesc || !m.market.input.Focused() {
		t.Fatalf("enter 应进入描述输入态并聚焦，实际 mode=%d focused=%v", m.market.mode, m.market.input.Focused())
	}
	m = driveKeys(t, m, "一", "个", "测", "试", "包", "#", "测", "试")
	if !strings.Contains(m.market.input.Value(), "#测试") {
		t.Fatalf("描述输入不符: %q", m.market.input.Value())
	}

	// enter → 令牌输入态（密码回显、聚焦）
	m = driveKeys(t, m, "enter")
	if m.market.mode != marketInputToken {
		t.Fatalf("enter 应进入令牌输入态，实际 mode=%d", m.market.mode)
	}
	if m.market.input.EchoMode != textinput.EchoPassword {
		t.Fatalf("令牌输入应为密码回显")
	}
	if !m.market.input.Focused() {
		t.Fatalf("令牌输入框应已聚焦")
	}

	// Esc 中途退出 → 回包选择且恢复明文模式（避免残焦与回显残留）
	m = driveKeys(t, m, "esc")
	if m.market.mode != marketSelectPkg {
		t.Fatalf("esc 应回包选择态，实际 mode=%d", m.market.mode)
	}
	if m.market.input.EchoMode != textinput.EchoNormal {
		t.Fatalf("退出令牌态应恢复明文回显")
	}
	if m.market.input.Focused() {
		t.Fatalf("退出输入态应已 Blur")
	}
	if m.market.desc == "" {
		t.Fatalf("已输入的描述应被保留（继续发布可复用）")
	}
	if m.market.archivePath != "" {
		t.Fatalf("目录发布源选中后 archivePath 应为空")
	}
}

// TestMarketArchiveUploadFlow 已打包 .ltpx 文件上传通道：f 输入路径 → 包内读元数据 → 描述 → 令牌
func TestMarketArchiveUploadFlow(t *testing.T) {
	// 构造琉璃导出格式的 .ltpx（zip 内容 + 顶层目录）
	archiveDir := t.TempDir()
	archivePath := filepath.Join(archiveDir, "假包.ltpx")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entries := map[string]string{
		"fake.pkg/index.html":    "<html></html>",
		"fake.pkg/metadata.json": `{"id":"fake.pkg","title":"假包","description":"测试用","tags":["T9"]}`,
	}
	for name, content := range entries {
		fw, fwErr := w.Create(name)
		if fwErr != nil {
			t.Fatal(fwErr)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// 注入测试配置（仓库指向不存在的属主，避免真实发布）
	pkgRoot := t.TempDir()
	SetMarketConfig(&ltpxmarket.Config{GiteeOwner: "test-none", GiteeRepo: "none", PackageDir: pkgRoot})
	defer SetMarketConfig(nil)

	m := newAppModel()
	m.width, m.height = 100, 30
	m.screen = screenMarket
	m.market.loaded = true

	// u → 包选择态 → f → 文件路径输入态（聚焦）
	m = driveKeys(t, m, "u")
	if m.market.mode != marketSelectPkg {
		t.Fatalf("u 后应进入包选择态，实际 mode=%d", m.market.mode)
	}
	m = driveKeys(t, m, "f")
	if m.market.mode != marketInputArchive || !m.market.input.Focused() {
		t.Fatalf("f 应进入文件路径输入态并聚焦，实际 mode=%d focused=%v", m.market.mode, m.market.input.Focused())
	}

	// 输入路径（逐字符）→ 回车读取元数据（命令同步执行，离线）
	m = driveKeys(t, m, toRunes(archivePath)...)
	m = driveKeys(t, m, "enter")
	if m.market.mode != marketInputDesc || !m.market.input.Focused() {
		t.Fatalf("读取成功后应进入描述输入态并聚焦，实际 mode=%d（err=%v）", m.market.mode, m.market.err)
	}
	if m.market.archivePath != archivePath {
		t.Fatalf("archivePath 应为 %q，实际 %q", archivePath, m.market.archivePath)
	}
	if m.market.archivePkg.Meta.ID != "fake.pkg" || m.market.archivePkg.Meta.Title != "假包" {
		t.Fatalf("包内元数据解析不符: %+v", m.market.archivePkg.Meta)
	}

	// 描述 → 令牌态（密码回显）→ Esc 退出（不提交，避免触达网络）
	m = driveKeys(t, m, "测", "试")
	m = driveKeys(t, m, "enter")
	if m.market.mode != marketInputToken || m.market.input.EchoMode != textinput.EchoPassword {
		t.Fatalf("enter 应进入密码回显的令牌态，实际 mode=%d", m.market.mode)
	}
	m = driveKeys(t, m, "esc")
	if m.market.mode != marketSelectPkg {
		t.Fatalf("esc 应回包选择态")
	}
	// 文件发布源保留：重新进入令牌提交仍走文件通道（此处仅断言状态未清空）
	if m.market.archivePath != archivePath {
		t.Fatalf("esc 退出发布不应清空文件发布源")
	}
}

// toRunes 把字符串拆成逐字符按键序列
func toRunes(s string) []string {
	out := make([]string, 0, len([]rune(s)))
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}
