package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/AudioTranscriber"
	"LunarSubsystem/LunarHarness/agents/DialogueBridging"
	"LunarSubsystem/LunarHarness/agents/MemoryFuser"
	"LunarSubsystem/LunarHarness/agents/MemoryTagger"
	"LunarSubsystem/LunarHarness/agents/Painter"
	"LunarSubsystem/LunarHarness/agents/filesupport"
	"LunarSubsystem/LunarHarness/agents/ltpxhost"
	"LunarSubsystem/LunarHarness/chain/search"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/clients"
	broadcaster "LunarSubsystem/LunarHarness/engine/broadcaster"
	"LunarSubsystem/LunarHarness/engine/browser"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logbuf"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/debugsnap"
	"LunarSubsystem/LunarHarness/engine/knowledge"
	"LunarSubsystem/LunarHarness/engine/library"
	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/protocolwire"
	"LunarSubsystem/LunarHarness/engine/resource"
	"LunarSubsystem/LunarHarness/engine/websocket"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
	"LunarSubsystem/LunarHarness/server"
)

// Assemble 装配各子系统：传输层 → HTTP 服务 → 服务层（月华生产者 / 琉璃）→ 消费层（Go 思维链）。
// 各层按依赖顺序初始化，跨层依赖（引擎上行观测、结构化消息桥接）在此注入，
// 避免包间反向依赖，亦避免各人格包互相 import。
// 消费层（月华思维链与 LTP9 插件沙箱）排在末段启动，
// 保证其思考循环/插件加载之时，HTTP、llama 代理、记忆库与 LTPX 协调层均已就绪。
func Assemble() {
	// ==== 模型调用计量：读回上次累计值 ====
	// 调用次数、上下文总量、缓存命中、峰值与平均输出保存在 local_data/database，
	// 重启后接着累加，而不是从零开始。
	if err := logbuf.LoadModelStats(); err != nil {
		logger.Warn("LunarHarness", "读取模型调用计量失败，本次从零累计: %v", err)
	}
	// ==== protocol 配置注入：必须在任何智能体调用模型之前 ====
	// protocol/agentbase 与 loopguard 只消费端点/阈值，不直接 import engine/config。
	protocolwire.Wire()
	// protocol/agentbase 的日志同样经注入：默认走标准库日志，这里改接统一日志层，
	// 保证 protocol 层既不直接 import engine/console/logger，又能留下同样的日志。
	agentbase.SetLogHook(func(level, message string) {
		switch level {
		case "warn":
			logger.Warn("CrystalAstral", "%s", message)
		case "error":
			logger.Error("CrystalAstral", "%s", message)
		default:
			logger.Info("CrystalAstral", "%s", message)
		}
	})

	// ==== 跨层注入：记忆与文件编辑 ====
	// 模型调用统一由独立智能体承载：engine 侧只保留装配钩子，不直接调用模型。
	memory.TagGenerator = MemoryTagger.Generate
	memory.MemoryMergeFunc = MemoryFuser.Merge
	resource.TranscribeAudioFunc = AudioTranscriber.Transcribe
	// 演绎链复用同一绘制智能体与参考图解析链路，不另起绘图实现。
	storyboard.PaintHook = func(ctx context.Context, req storyboard.PaintRequest) (storyboard.PaintResult, error) {
		refs, err := thinking.PainterReferences(req.References)
		if err != nil {
			return storyboard.PaintResult{}, fmt.Errorf("参考资源解析失败：%w", err)
		}
		reference := &Painter.Reference{Explicit: true}
		if len(refs) > 0 {
			reference.Handle = refs[0].Handle
			reference.Source = refs[0].Source
			reference.Desc = refs[0].Desc
			reference.Extras = refs[1:]
		}
		paintReq := Painter.InstructionRequest{
			Prompt:     req.Prompt,
			Ratio:      req.Ratio,
			Style:      req.Style,
			Change:     req.Change,
			Output:     req.Output,
			Task:       req.Task,
			References: append([]string(nil), req.References...),
		}
		// 演绎链按需静默：只登记图片资源，不推送到月华前端、不写记忆、不发 QQ。
		var env Painter.Env = thinking.PainterHost{}
		if req.Silent {
			env = silentPainterHost{}
		}
		result, err := Painter.Perform(ctx, Painter.BuildInstruction(paintReq), reference, env,
			Painter.Options{Silent: req.Silent, OutputFolder: req.OutputFolder})
		if err != nil {
			return storyboard.PaintResult{Summary: result.Summary}, err
		}
		if len(result.Handles) == 0 {
			return storyboard.PaintResult{Summary: result.Summary},
				fmt.Errorf("图片绘制未产出资源句柄：%s", clipPaintReason(result.Summary))
		}
		return storyboard.PaintResult{Summary: result.Summary, Handles: result.Handles}, nil
	}
	// 演绎链的资料准备阶段复用现有网络搜索能力。
	storyboard.WebSearchHook = func(query string) (string, error) {
		return search.Run("请检索并简明总结以下主题，重点给出可直接用于画面设定的事实：\n" + query)
	}
	// 演绎任务状态变化同样推送到资源库前端。
	storyboard.TaskEventHook = func(view storyboard.TaskView) {
		if websocket.Instance != nil {
			websocket.Instance.TryBroadcastMessageToChannel(libraryTerminalChannel, "storyboard_task", view)
		}
	}
	// 对话者工具执行器已归 agents/Dialogue，生活 / 检索能力由 thinking 装配期注入。

	// ==== 跨层注入：HTTP 层所需能力 ====
	// 必须在 server.Start 之前接线：消息写入 / 代理 / 视觉读取 / 外部推理排队。
	wireServerHooks()

	// ==== 开发者模式告警快照：日志钩子装配 ====
	// 必须在其他子系统之前接上：装配期出现的警告也应留下调试快照。
	wireDebugSnapshot()

	// ==== 传输层：统一 WebSocket 集线器 ====
	websocket.Instance = websocket.NewHub()
	go websocket.Instance.Run()
	// 资源库前端的命令通道：上行命令与下行回执都在 /ws?channel=library 内完成。
	websocket.ClientMessageHandler = server.HandleLibraryWSMessage
	logger.Info("LunarHarness", "统一 WebSocket 集线器已启动")

	// ==== 传输层：结构化广播 ====
	// 动作匹配由前端渲染引擎自行完成（嵌入模型 + 动作列表），后端不再观测引擎上行帧。

	// ==== 跨层注入：结构化广播 & 下行桥接 ====
	// 智能体下行走结构化广播（type/data），QQ 桥接挂在结构化旁路上（原始字节广播不触发）
	thinking.PushMessageFunc = websocket.Instance.BroadcastMessage
	websocket.OnStructuredMessage = DialogueBridging.BridgeStructuredMessage

	// ==== 跨层注入：资源库编辑状态实时推送 ====
	// 文件编辑智能体每次操作都会写入资源库变更库，这里把同步信号只推给资源库前端。
	// 必须按频道定向投递：library_sync 是编辑器状态事件，不是聊天消息；
	// 若走全量广播，消息终端会把无法识别的结构体当成内容渲染到聊天记录里。
	if err := library.Init(); err != nil {
		logger.Warn("LunarHarness", "资源库变更库初始化失败: %v", err)
	}
	library.SetBroadcast(func(msgType string, data any) {
		websocket.Instance.BroadcastMessageToChannel(libraryTerminalChannel, msgType, data)
	})

	// ==== 前端静态资源：消息终端 / 移动端 / 琉璃桌面 / 资源库 ====
	StaticRootsProvider = func() server.StaticRoots {
		return server.StaticRoots{
			Lunar:    lunarStaticRoot(),
			Mobile:   clients.MobileFS(),
			Crystal:  crystalStaticRoot(),
			Library:  clients.LibraryFS(),
			Reliance: clients.RelianceFS(),
			Shared:   clients.SharedFS(),
		}
	}

	// ==== HTTP 服务：单端口承载两个前端与全部端点 ====
	// 必须在人格层之前启动：月华的智能体运行时一经装配即开始运行思考链，
	// 其内部会经 syncFetch 回调本机端点（/vram/unload、/vram/guard、/v1 等），
	// 若此时监听尚未就绪，这些回调会因连接被拒而失败。
	srv, err := server.Start(StaticRootsProvider())
	if err != nil {
		// 启动失败与警告统一由日志层播放失败音效（logger.Fatal 会同步播完再退出）
		logger.Fatal("LunarHarness", "服务启动失败: %v", err)
	}
	httpServer = srv

	// ==== 人格层：月华 ====
	if err := assembleLunar(); err != nil {
		logger.Fatal("LunarHarness", "月华人格局装配失败: %v", err)
	}
	logger.Info("LunarHarness", "月华人格局装配完成")

	// ==== 人格层：琉璃 ====
	if err := assembleCrystal(); err != nil {
		logger.Fatal("LunarHarness", "琉璃人格格装配失败: %v", err)
	}
	logger.Info("LunarHarness", "琉璃人格格装配完成")

	// ==== 琉璃前端守护：LTPX 派发的前端依赖 ====
	// 月华的机制运行不依赖前端（前端仅显示与记录），但琉璃与 LTPX 的功能实现依托前端
	// （ltpx_call 派发与 LTP9 插件沙箱都在琉璃桌面页面执行）。LTPX 调用发生而琉璃前端
	// 已被关闭时，守护经唤起器重建桌面窗口，并等待其重新接入总线后再广播。
	ltpxhost.SetFrontendRelauncher(func() error {
		// 重启语义：窗口可能残留死条目（Run 未返回）或页面已断连，需重载/重建
		return browser.RestartDesktopWindow(crystalWindowSpec())
	})
	ltpxhost.SetFrontendReadyProbe(func() bool {
		return websocket.Instance != nil && websocket.Instance.ChannelBusCount(websocket.DefaultBusChannel) > 0
	})

	// ==== TTS 闲置卸载守护：在位引擎 5 分钟无合成调用即卸载模型占用 ====
	// 引擎本体在首次合成请求时经统一播报链路（engine/broadcaster）惰性加载，本守护只负责闲置回收。
	broadcaster.StartIdleUnloader()

	// ==== 消费层启动：月华 Go 原生思维链（每秒思考循环） ====
	// 必须最后启动：思考循环一经运行即可能消费愿望到期等任务，若此时 llama 模型、
	// 记忆库、LTP9/LTPX 协调层尚未就绪，到期任务会在依赖缺失下执行。
	if err := startLunarAgent(); err != nil {
		logger.Fatal("LunarHarness", "月华智能体运行时启动失败: %v", err)
	}
	logger.Info("LunarHarness", "月华智能体运行时（Go 思维链）已启动")
}

// clipPaintReason 截断绘图失败原因，避免错误日志与任务状态过长。
func clipPaintReason(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "图片绘制智能体没有返回任何结果"
	}
	runes := []rune(text)
	if len(runes) > 300 {
		return string(runes[:300]) + "…"
	}
	return text
}

// silentPainterHost 演绎链专用的静默宿主：
// 仍登记图片资源并返回可查看地址，但跳过前端推送、近期记忆写入与 QQ 通知。
type silentPainterHost struct {
	thinking.PainterHost
}

func (silentPainterHost) PushImages(images []string) []string {
	return thinking.RegisterPainterImages(images)
}

func (silentPainterHost) PushImagesSilently(images []string) []string {
	return thinking.RegisterPainterImages(images)
}

// wireDebugSnapshot 装配开发者模式下的告警调试快照。
//
// 触发条件由日志层把守（developer 模式 + WARN 及以上），本函数只负责接上钩子；
// 快照实现（engine/debugsnap）只读取日志缓冲 / 实况轮次 / 智能体上下文并落盘，
// 调用链中不包含任何前端窗口打开逻辑（不弹月华/琉璃界面）。
func wireDebugSnapshot() {
	logger.SetDevMode(config.Developer)
	logger.SetAlertHook(debugsnap.HandleAlert)
}

// lunarWindowSpec 月华消息终端窗口规格
func lunarWindowSpec() browser.DesktopWindowSpec {
	base := fmt.Sprintf("http://127.0.0.1:%d", config.BasicPort)
	return browser.DesktopWindowSpec{
		ID:       WindowLunar,
		URL:      base + "/",
		Title:    "[星月智能] : 钛宇-月华",
		Width:    lunarWindowWidth,
		Height:   lunarWindowHeight,
		IconPath: lunarIconPath,
		// 缩到移动端尺寸时置顶，避免被其它程序遮挡（桌面宠物式常驻）
		StickyWhenNarrow: true,
	}
}

// crystalWindowSpec 琉璃桌面窗口规格（启动打开与 LTPX 前端守护唤起共用）
func crystalWindowSpec() browser.DesktopWindowSpec {
	base := fmt.Sprintf("http://127.0.0.1:%d", config.BasicPort)
	return browser.DesktopWindowSpec{
		ID:       WindowCrystal,
		URL:      base + server.DesktopPrefix,
		Title:    "[星月智能] : 钛宇-琉璃",
		Width:    crystalWindowWidth,
		Height:   crystalWindowHeight,
		IconPath: crystalIconPath,
		// 琉璃桌面被外部链接 _self 跳转走后，仍可经悬浮按钮回到琉璃主页面
		InitJS: browser.BuildReturnButtonJS(base + server.DesktopPrefix),
	}
}

// libraryWindowSpec 资源库窗口规格（按需弹出；控制台「前端窗口」也可唤起）
func libraryWindowSpec() browser.DesktopWindowSpec {
	base := fmt.Sprintf("http://127.0.0.1:%d", config.BasicPort)
	return browser.DesktopWindowSpec{
		ID:     WindowLibrary,
		URL:    base + server.LibraryPrefix,
		Title:  "[星月智能] : 资源库",
		Width:  1180,
		Height: 820,
	}
}

// OpenFrontendWindows 打开两个常驻前端窗口：月华消息终端（/）与琉璃桌面（/desktop）。
//
// 控制台（TUI）是程序主界面，这两个窗口是它的可视化前端：窗口全部关闭不会退出程序，
// 用户可在控制台「运行状态」屏的前端窗口区块重新唤起（入口键 1 / 2）。
// 资源库不随程序自动弹出，只在运行浏览器端代码或用户显式唤起时打开（控制台入口键 3）。
func OpenFrontendWindows() {
	for _, spec := range []browser.DesktopWindowSpec{lunarWindowSpec(), crystalWindowSpec()} {
		if err := browser.OpenDesktopWindow(spec); err != nil {
			logger.Error("LunarHarness", "前端窗口 [%s] 打开失败: %v", spec.ID, err)
		}
	}
}

// GracefulShutdown 优雅关闭全部子系统（控制台退出后由 main 调用）：
// 关闭顺序：前端窗口 → HTTP 服务 → 人格层资源 → WebSocket 集线器。
// 优雅关闭放在独立 goroutine 并设兜底超时：卡在 webview Run() 中的窗口
// （见 engine/browser/desktop.go 文件头）可能使关闭无法及时返回，超时后交由 main 的 os.Exit(0) 收尾。
func GracefulShutdown(reason string) {
	logger.Info("LunarHarness", "%s，正在关闭...", reason)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// 模型调用计量：先落盘再关子系统，后续关闭即便卡住也不会丢掉累计值
		if err := logbuf.SaveModelStats(); err != nil {
			logger.Warn("LunarHarness", "保存模型调用计量失败: %v", err)
		}
		browser.CloseAllDesktopWindows()
		server.Shutdown(httpServer)
		shutdownLunar()
		// 统一资源目录：关闭 SQLite 连接（内容仓库中的 blob 不动，句柄持久保留）
		if err := resource.Close(); err != nil {
			logger.Warn("LunarHarness", "关闭资源目录失败: %v", err)
		}
		// 知识库与领域库（media / web / text / character）一并释放
		if err := knowledge.Close(); err != nil {
			logger.Warn("LunarHarness", "关闭知识库与领域数据库失败: %v", err)
		}
		// 资源库变更库（library.db）
		if err := library.Close(); err != nil {
			logger.Warn("LunarHarness", "关闭资源库变更库失败: %v", err)
		}
		// 文件版本库（versions.db）
		if err := filesupport.Close(); err != nil {
			logger.Warn("LunarHarness", "关闭文件版本库失败: %v", err)
		}
		if websocket.Instance != nil {
			websocket.Instance.CloseAll()
		}
	}()

	select {
	case <-done:
		logger.Info("LunarHarness", "已安全关闭")
	case <-time.After(shutdownTimeout):
		logger.Warn("LunarHarness", "优雅关闭超时（%s），强制退出", shutdownTimeout)
	}
}
