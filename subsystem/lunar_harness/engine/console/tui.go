package console

// tui.go 控制台 TUI 的模型定义与入口装配。
// 事件与输入处理见 tui_update.go，启动/清理见 tui_run.go，渲染分发见 tui_view.go。

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"LunarSubsystem/LunarHarness/engine/console/proxy"
	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// ==== 消息定义 ====

// notifyMsg 日志管线唤醒信号（日志/流事件到达）
type notifyMsg struct{}

// statsTickMsg 运行状态采样刷新
type statsTickMsg struct{}

// streamEvtMsg 智能体流式事件
type streamEvtMsg struct{ evt StreamEvent }

// taskDoneMsg 后台任务完成（ports/proxy 等共用）
type taskDoneMsg struct{ err error }

// progressMsg 任务进度（-1 表示准备阶段未知进度）
type progressMsg int

// taskLogMsg 功能任务日志行（功能屏内嵌回显）
type taskLogMsg string

// frontendDoneMsg 前端唤起任务完成
type frontendDoneMsg struct {
	which string // "lunar" / "crystal"
	err   error
}

// proxyReadyMsg 代理服务启动成功
type proxyReadyMsg struct {
	info  proxy.Info
	token *proxy.Handle
}

// proxyFailMsg 代理服务启动失败
type proxyFailMsg struct{ err error }

// proxyStoppedMsg 代理服务已停止（后台收尾完成；离开代理屏后仍需生效，故在顶层处理）
type proxyStoppedMsg struct{}

// ==== 屏幕编号 ====

type screenID int

const (
	screenMenu screenID = iota
	screenAgent
	screenStats
	screenLogs
	screenProxy
	screenArchive
	screenPorts
	screenDump
	screenTools
)

// menuItem 主菜单条目：快捷键、分组与目标屏幕均显式声明。
// 进入哪个屏由 screen 字段决定（而非下标推算），故调整排版或条目顺序
// 都不会造成「显示与实际功能对应不上」。
type menuItem struct {
	key    string   // 快捷键标签（单字符可直接按；空串=仅回车进入；退出项为 "X"）
	title  string   // 条目名称
	desc   string   // 条目说明（宽屏显示于右侧详情栏）
	group  string   // 分组（渲染时在该组首条之前插入分隔标题）
	screen screenID // 目标屏幕；值为 screenMenu 时表示退出程序（哨兵）
}

// menuItems 主菜单条目表：顺序即显示顺序，按「运行观测 / 系统管理 / 退出」
// 分组组织：先用得上的是观测与运维，退出沉到最后。
//
// 功能屏的快捷键按显示顺序依次占用 0-9：用户只需记住「第几个」即可，不必再记
// 每屏各自的历史助记键。新增条目时按顺序追加，不要插空或跳号。
// 末项「退出程序」是哨兵，不占项号——界面统一显示 X（大小写 x/X 都可按），
// 与全局退出键一致，避免出现「标签是 8、实际按 x」两套东西不同步。
//
// 更新（打包上传 / 下载安装）已整体迁移到独立程序 subsystem/update_applier，主程序不再提供更新入口。
// 「打包归档」是主程序的本地留存能力，与更新包互不相关：只把当前程序与资源打成 7z 分卷放在本地，
// 不上传、不安装、不参与版本比对；打包期间会暂停思维链并卸载对话模型。
var menuItems = []menuItem{
	{key: "0", title: "推理实况", group: "运行观测", screen: screenAgent,
		desc: "月华思考 / 输出 / 工具调用的流式显示"},
	{key: "1", title: "运行状态", group: "运行观测", screen: screenStats,
		desc: "运行环境 / 模型计量 / 前端窗口 / " + agentinfo.NameDesktopOperator + "；状态按绿黄红区分"},
	{key: "2", title: "系统日志", group: "运行观测", screen: screenLogs,
		desc: "统一日志屏：左右方向键切换 原始 / 警告 / 绘图 / 对话 / 生活 / 后勤"},
	{key: "3", title: "数据导出", group: "运行观测", screen: screenDump,
		desc: "导出原始日志、推理实况、以及 Go 智能体上下文"},

	{key: "4", title: "工具列表", group: "系统管理", screen: screenTools,
		desc: "扫描并展示全部 Go 智能体的工具与说明"},
	{key: "5", title: "局域共享", group: "系统管理", screen: screenProxy,
		desc: "WSS → WS 反向代理，支持 IP 选择与手机扫码接入"},
	{key: "6", title: "打包归档", group: "系统管理", screen: screenArchive,
		desc: "本地 7z 分卷留存（不发布、不安装）；打包期间暂停思维链并卸载对话模型"},
	{key: "7", title: "端口释放", group: "系统管理", screen: screenPorts,
		desc: "扫描指定端口范围，终止占用进程并验证释放"},

	{key: "X", title: "退出程序", group: "退出", screen: screenMenu,
		desc: "关闭控制台并优雅停止全部子系统（感谢使用「星月智能 · 控制台」）"},
}

// menuHotkeyIndex 快捷键 → 菜单索引（未命中返回 -1）
func menuHotkeyIndex(k string) int {
	for i, it := range menuItems {
		if it.key != "" && it.key == k {
			return i
		}
	}
	return -1
}

// ==== 应用主模型 ====

// appModel 全应用模型：屏幕切换 + 各屏状态聚合 + 框体级状态
type appModel struct {
	screen screenID
	menu   int // 主菜单选中项
	menuPl listPane

	width, height int
	help          bool // 键位帮助页（模态）

	// 任务指示：标题栏承载「进行中 / 刚结束」的跨屏反馈，
	// 使用户离开功能屏后仍能看见任务进展与结果
	busy      string
	notice    string
	noticeErr bool
	noticeAt  time.Time

	spinner spinner.Model

	// 系统日志：单一屏 + 分类（左右方向键切换）
	logCat   LogSource
	logView  logsState
	agent    agentState
	stats    statsState
	frontend frontendState
	proxy    proxyState
	archive  archiveState
	ports    portsState
	dump     dumpState
	engine   engineState
	tools    toolsState

	streamUnsub func() // 流式事件订阅注销
	sizeLogged  bool   // 首帧尺寸自检是否已落盘
}

// newAppModel 构建初始模型
func newAppModel() appModel {
	return appModel{
		screen:  screenMenu,
		spinner: newSpinner(),
		agent:   agentState{follow: true}, // 实况屏默认跟随最新轮次
		logCat:  SourceRaw,
		logView: newLogsState(SourceRaw),
		proxy:   newProxyState(),
		archive: newArchiveState(),
		ports:   newPortsState(),
	}
}

// newSpinner 创建加载动画
func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = selectStyle
	return s
}

// Init 初始化：启动状态采样 tick 与加载动画；配置开启时用默认参数自动启动 HTTPS 代理
func (m appModel) Init() tea.Cmd {
	cmds := []tea.Cmd{tickStats(), m.spinner.Tick}
	if autoProxyStart {
		// 默认参数：端口取代理屏预填值（BasicPort / ProxyPort），绑定首个本机 IP
		m.proxy.ips = proxy.LocalIPs()
		m.proxy.ipIdx = 0
		cmds = append(cmds, autoStartProxyCmd(m.proxy))
	}
	return tea.Batch(cmds...)
}

// tickStats 生成状态刷新命令（同时驱动标题栏时钟与提示超时）
func tickStats() tea.Cmd {
	return tea.Every(statsInterval, func(time.Time) tea.Msg {
		return statsTickMsg{}
	})
}
