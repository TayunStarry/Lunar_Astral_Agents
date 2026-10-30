package console

// ==== 调色板与样式 ====
//
// 统一色阶：品牌紫承担标题与选中态，青绿承担数值与成功，橙/红承担警告与错误，
// 灰阶承担结构线（边框、分隔）与次要信息。各屏共用同一色板，
// 避免「每个屏各自取色」造成的观感漂移。
//
// 色值全部是 **256 色索引**：中文 Windows 的经典 conhost 对 24 位色的支持随版本波动，
// 而索引色在 conhost / Windows Terminal / 远程 SSH 里表现一致。

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	colAccent     = lipgloss.Color("213") // 品牌紫：标题、选中光标
	colAccentSoft = lipgloss.Color("219") // 浅紫：键位提示
	colText       = lipgloss.Color("252") // 正文
	colValue      = lipgloss.Color("86")  // 数值/强调（青）
	colDim        = lipgloss.Color("245") // 次要信息
	colFaint      = lipgloss.Color("240") // 更次要（占位、空态）
	colRule       = lipgloss.Color("238") // 结构线
	colOK         = lipgloss.Color("82")  // 成功
	colWarn       = lipgloss.Color("214") // 警告
	colErr        = lipgloss.Color("204") // 错误
	colSelBg      = lipgloss.Color("237") // 选中行底色
	colSelFg      = lipgloss.Color("231") // 选中行前景
	colSelDim     = lipgloss.Color("250") // 选中行内的次级文字

	colLogInfo  = lipgloss.Color("255") // 日志：常规（白色）
	colLogDim   = lipgloss.Color("245") // 日志：stdout 原始行
	colLogProxy = lipgloss.Color("39")  // 日志：任务回显
	colThink    = lipgloss.Color("243") // 思考流（暗色）
	colRole     = lipgloss.Color("176") // 智能体角色名
	colDraw     = lipgloss.Color("51")  // 绘图采样进度
	colEditWarm = lipgloss.Color("226") // 编辑显示：本次运行触碰的文件（黄）
	colEditLive = lipgloss.Color("48")  // 编辑显示：最近改动片段（绿）
	colEditBase = lipgloss.Color("39")  // 编辑显示：普通文件名（蓝）
	colTokenIn  = lipgloss.Color("39")  // 模型 token：输入（蓝色系）
	colTokenOut = lipgloss.Color("213") // 模型 token：输出（粉色系）
)

// ==== 基础样式 ====
//
// 注意：这些变量**没有初值**，必须由 bindTheme 绑定（见下方说明）。
// 零值 lipgloss.Style 仍可安全 Render（只输出纯文本），因此漏绑不会 panic，
// 只会「没颜色」——这正是历史缺陷的表现形式。

var (
	titleStyle   lipgloss.Style // 标题栏品牌
	crumbStyle   lipgloss.Style // 标题栏面包屑（上一级）
	labelStyle   lipgloss.Style // 正文
	valueStyle   lipgloss.Style // 数值/强调
	dimStyle     lipgloss.Style // 次要信息
	faintStyle   lipgloss.Style // 更次要
	selectStyle  lipgloss.Style // 选中/当前
	okStyle      lipgloss.Style // 成功
	warnStyle    lipgloss.Style // 警告
	errStyle     lipgloss.Style // 错误
	cursorStyle  lipgloss.Style // 列表光标
	ruleStyle    lipgloss.Style // 结构线
	keyStyle     lipgloss.Style // 键位
	hintStyle    lipgloss.Style // 键位说明
	sectionStyle lipgloss.Style // 区块标题
	selRowStyle  lipgloss.Style // 选中行（整行底色）
	selDimStyle  lipgloss.Style // 选中行内的次级文字

	navGroupStyle lipgloss.Style // 侧栏分组标题
	navItemStyle  lipgloss.Style // 侧栏条目
	navKeyStyle   lipgloss.Style // 侧栏条目的快捷键标记
)

// ==== 日志专用样式 ====

var (
	logInfoStyle      lipgloss.Style // 青色：常规
	logDimStyle       lipgloss.Style // 灰色：stdout 原始行
	logProxyStyle     lipgloss.Style // 蓝色：任务回显
	logWarnStyle      lipgloss.Style // 橙色：警告
	logErrStyle       lipgloss.Style // 红色：错误
	thinkStyle        lipgloss.Style // 思考流（暗色）
	thinkHeadStyle    lipgloss.Style // 思考/输出分区标题
	roleStyle         lipgloss.Style // 智能体角色名
	drawProgressStyle lipgloss.Style // 绘图采样进度
	editTouchedStyle  lipgloss.Style // 编辑显示：本次运行触碰的文件名（黄）
	editFileStyle     lipgloss.Style // 编辑显示：普通文件名（蓝）
	editFailStyle     lipgloss.Style // 编辑显示：最近运行失败的文件名（红）
	editHiliteStyle   lipgloss.Style // 编辑显示：最近改动片段（绿）
	editLineNoStyle   lipgloss.Style // 编辑显示：行号（灰）
	editRunStyle      lipgloss.Style // 编辑显示：运行入口提示（紫）
	tokenInStyle      lipgloss.Style // 模型 token：输入（蓝色系）
	tokenOutStyle     lipgloss.Style // 模型 token：输出（粉色系）
)

// bindTheme 把全部样式（重新）绑定到渲染器 r。
//
// 为什么必须显式重绑：lipgloss 的 `Style` 是值类型，`NewStyle()` 当场把渲染器指针存进
// `Style.r`；此后 `SetDefaultRenderer` 只替换包级变量，**已建样式仍指向旧渲染器**。
// 而旧渲染器的输出正是被 Install() 接到日志管道上的 `os.Stdout`——它的 isatty 判定为假，
// 颜色档位退化为 Ascii，整套配色被丢弃，界面于是「一片纯白」。详见 renderer.go 与
// docs/adr/0002-控制台渲染器的颜色档位与样式绑定.md。
//
// 因此：**任何新增样式都必须写进本函数**，不要在包级做 `lipgloss.NewStyle()` 初始化；
// 单测 `TestBindThemeRebindsAllStyles` 会拦住漏绑。
func bindTheme(r *lipgloss.Renderer) {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	ns := func() lipgloss.Style { return lipgloss.NewStyle().Renderer(r) }

	titleStyle = ns().Bold(true).Foreground(colAccent)
	crumbStyle = ns().Foreground(colDim)
	labelStyle = ns().Foreground(colText)
	valueStyle = ns().Bold(true).Foreground(colValue)
	dimStyle = ns().Foreground(colDim)
	faintStyle = ns().Foreground(colFaint)
	selectStyle = ns().Bold(true).Foreground(colAccent)
	okStyle = ns().Bold(true).Foreground(colOK)
	warnStyle = ns().Foreground(colWarn)
	errStyle = ns().Bold(true).Foreground(colErr)
	cursorStyle = ns().Bold(true).Foreground(colAccent)
	ruleStyle = ns().Foreground(colRule)
	keyStyle = ns().Bold(true).Foreground(colAccentSoft)
	hintStyle = ns().Foreground(colDim)
	sectionStyle = ns().Bold(true).Foreground(colValue)
	selRowStyle = ns().Background(colSelBg).Foreground(colSelFg)
	// 选中行内的次级文字：底色之上仍保持可读的灰
	selDimStyle = ns().Background(colSelBg).Foreground(colSelDim)

	navGroupStyle = ns().Bold(true).Foreground(colDim)
	navItemStyle = ns().Foreground(colText)
	navKeyStyle = ns().Foreground(colFaint)

	logInfoStyle = ns().Foreground(colLogInfo)
	logDimStyle = ns().Foreground(colLogDim)
	logProxyStyle = ns().Foreground(colLogProxy)
	logWarnStyle = ns().Foreground(colWarn)
	logErrStyle = ns().Foreground(colErr)
	thinkStyle = ns().Foreground(colThink)
	thinkHeadStyle = ns().Bold(true).Foreground(colDim)
	roleStyle = ns().Bold(true).Foreground(colRole)
	drawProgressStyle = ns().Foreground(colDraw)
	editTouchedStyle = ns().Foreground(colEditWarm)
	editFileStyle = ns().Foreground(colEditBase)
	editFailStyle = ns().Bold(true).Foreground(colErr)
	editHiliteStyle = ns().Foreground(colEditLive)
	editLineNoStyle = ns().Foreground(colDim)
	editRunStyle = ns().Bold(true).Foreground(colAccentSoft)
	tokenInStyle = ns().Bold(true).Foreground(colTokenIn)
	tokenOutStyle = ns().Bold(true).Foreground(colTokenOut)
}

// init 让样式在「未调用 Run 的场合」（单测、宿主嵌入渲染）也可用：
// 绑定到默认渲染器。Run 拿到真实控制台句柄后会再次 bindTheme 覆盖它。
func init() {
	bindTheme(lipgloss.DefaultRenderer())
}

// renderLogLine 单条日志渲染：按级别/来源着色
func renderLogLine(line LogLine) string {
	switch {
	case line.Level == "ERROR" || line.Level == "FATAL":
		return logErrStyle.Render(line.Text)
	case line.Level == "WARN":
		return logWarnStyle.Render(line.Text)
	case line.Level == "RAW":
		return logDimStyle.Render(line.Text)
	case line.Source == "task":
		return logProxyStyle.Render(line.Text)
	default:
		return logInfoStyle.Render(line.Text)
	}
}
