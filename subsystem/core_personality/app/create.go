package app

import (
	"fmt"
	"time"

	"CorePersonality/crystal"
	"CorePersonality/engine/lunar_loop"
	"CorePersonality/engine/tts_router"
	"CorePersonality/server"
	"CorePersonality/transport/enginebus"
	"CorePersonality/transport/napcat"
	"CorePersonality/transport/ws"
	"LunarSubsystem/BrowserClient"
	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// Assemble 装配各子系统：传输层 → HTTP 服务 → 服务层（月华生产者 / 琉璃）→ 消费层（goja 运行时）。
// 各层按依赖顺序初始化，跨层依赖（引擎总线发送函数、结构化消息桥接）在此注入，
// 避免包间反向依赖，亦避免各人格包互相 import。
// 消费层（月华智能体运行时与 LTP9 插件沙箱）的 goja 运行时排在末段启动，
// 保证其思考循环/插件加载之时，HTTP、llama 代理、记忆库与 LTPX 协调层均已就绪。
func Assemble() {
	// ==== 传输层：统一 WebSocket 集线器 ====
	ws.Instance = ws.NewHub()
	go ws.Instance.Run()
	LoggerGeneral.Info("CorePersonality", "统一 WebSocket 集线器已启动")

	// ==== 传输层：引擎消息总线（反射观测 + 原样透传） ====
	enginebus.Instance = enginebus.NewBus()
	enginebus.Instance.SetSender(func(data []byte) {
		if ws.Instance != nil {
			ws.Instance.BroadcastBus(ws.DefaultBusChannel, data)
		}
	})
	// 注册引擎消息观察者：方法名 OnXxx 反射映射到消息 type（OnAnimationList → animation_list）
	enginebus.Instance.Subscribe(lunarloop.AnimationObserver{})
	enginebus.Instance.Subscribe(lunarloop.TelemetryObserver{})
	LoggerGeneral.Info("CorePersonality", "引擎消息总线（反射 + 透传）已启动")

	// ==== 跨层注入：结构化广播 & 下行桥接 ====
	// 智能体下行走结构化广播（type/data），QQ 桥接挂在结构化旁路上（原始字节广播不触发）
	lunarloop.PushMessageFunc = ws.Instance.BroadcastMessage
	ws.OnStructuredMessage = napcat.BridgeStructuredMessage

	// ==== 前端静态资源：月华消息终端（/）与琉璃桌面（/desktop/） ====
	StaticRootsProvider = func() server.StaticRoots {
		return server.StaticRoots{
			Lunar:   lunarStaticRoot(),
			Crystal: crystalStaticRoot(),
		}
	}

	// ==== HTTP 服务：单端口承载两个前端与全部端点 ====
	// 必须在人格层之前启动：月华的智能体运行时一经装配即开始运行思考链，
	// 其内部会经 syncFetch 回调本机端点（/vram/unload、/vram/guard、/v1 等），
	// 若此时监听尚未就绪，这些回调会因连接被拒而失败。
	srv, err := server.Start(StaticRootsProvider())
	if err != nil {
		// Windows 桌面构建隐藏控制台，启动失败以失败音效提示（同时落日志）
		PlayStartupFailSound()
		LoggerGeneral.Fatal("CorePersonality", "服务启动失败: %v", err)
	}
	httpServer = srv

	// ==== 人格层：月华 ====
	if err := assembleLunar(); err != nil {
		LoggerGeneral.Fatal("CorePersonality", "月华人格局装配失败: %v", err)
	}
	LoggerGeneral.Info("CorePersonality", "月华人格局装配完成")

	// ==== 人格层：琉璃 ====
	if err := assembleCrystal(); err != nil {
		LoggerGeneral.Fatal("CorePersonality", "琉璃人格格装配失败: %v", err)
	}
	LoggerGeneral.Info("CorePersonality", "琉璃人格格装配完成")

	// ==== 琉璃前端守护：LTPX 派发的前端依赖 ====
	// 月华的机制运行不依赖前端（前端仅显示与记录），但琉璃与 LTPX 的功能实现依托前端
	// （ltpx_call 派发与 LTP9 插件沙箱都在琉璃桌面页面执行）。LTPX 调用发生而琉璃前端
	// 已被关闭时，守护经唤起器重建桌面窗口，并等待其重新接入总线后再广播。
	crystal.SetFrontendRelauncher(func() error {
		// 重启语义：窗口可能残留死条目（Run 未返回）或页面已断连，需重载/重建
		return BrowserClient.RestartDesktopWindow(crystalWindowSpec())
	})
	crystal.SetFrontendReadyProbe(func() bool {
		return ws.Instance != nil && ws.Instance.ChannelBusCount(ws.DefaultBusChannel) > 0
	})

	// ==== TTS 闲置卸载守护：三引擎 5 分钟无合成调用即卸载模型占用 ====
	// 引擎本体在首次合成请求时惰性加载（engine/tts_router），本守护只负责闲置回收。
	ttsrouter.StartIdleUnloader()

	// ==== 统一资源仓库回收守护：按配额与保留等级回收 blob（句柄保留，降级为描述） ====
	startArchiveCollector()

	// ==== 入站消费：ltp9/* 调试信封交由 LTP9 引擎处理 ====
	ws.Instance.SubscribeInbound("ltp9", crystal.HandleInbound)

	// ==== 消费层启动：月华智能体运行时（goja + agentSystem.js，每秒思考循环） ====
	// 必须最后启动：思考循环一经运行即可能消费计划表等到期任务，若此时 llama 模型、
	// 记忆库、LTP9/LTPX 协调层尚未就绪，到期任务会在依赖缺失下执行，导致 llama 代理
	// 报错与月华智能体事件循环长时间阻塞。
	if err := startLunarAgent(); err != nil {
		LoggerGeneral.Fatal("CorePersonality", "月华智能体运行时启动失败: %v", err)
	}
	LoggerGeneral.Info("CorePersonality", "月华智能体运行时（goja）已启动")
}

// lunarWindowSpec 月华消息终端窗口规格
func lunarWindowSpec() BrowserClient.DesktopWindowSpec {
	base := fmt.Sprintf("http://127.0.0.1:%d", *GeneralConfig.BasicPort)
	return BrowserClient.DesktopWindowSpec{
		ID:       WindowLunar,
		URL:      base + "/",
		Title:    "[星月智能] : 钛宇-月华",
		Width:    lunarWindowWidth,
		Height:   lunarWindowHeight,
		IconPath: lunarIconPath,
	}
}

// crystalWindowSpec 琉璃桌面窗口规格（启动打开与 LTPX 前端守护唤起共用）
func crystalWindowSpec() BrowserClient.DesktopWindowSpec {
	base := fmt.Sprintf("http://127.0.0.1:%d", *GeneralConfig.BasicPort)
	return BrowserClient.DesktopWindowSpec{
		ID:       WindowCrystal,
		URL:      base + server.DesktopPrefix,
		Title:    "[星月智能] : 钛宇-琉璃",
		Width:    crystalWindowWidth,
		Height:   crystalWindowHeight,
		IconPath: crystalIconPath,
		// 琉璃桌面被外部链接 _self 跳转走后，仍可经悬浮按钮回到琉璃主页面
		InitJS: BrowserClient.BuildReturnButtonJS(base + server.DesktopPrefix),
	}
}

// OpenFrontendWindows 打开两个前端窗口：月华消息终端（/）与琉璃桌面（/desktop）。
//
// 控制台（TUI）是程序主界面，这两个窗口是它的可视化前端：窗口全部关闭不会退出程序，
// 用户可在控制台「前端页面 管理」中重新唤起（见 tui_system 的前端管理屏）。
func OpenFrontendWindows() {
	for _, spec := range []BrowserClient.DesktopWindowSpec{lunarWindowSpec(), crystalWindowSpec()} {
		if err := BrowserClient.OpenDesktopWindow(spec); err != nil {
			LoggerGeneral.Error("CorePersonality", "前端窗口 [%s] 打开失败: %v", spec.ID, err)
		}
	}
}

// GracefulShutdown 优雅关闭全部子系统（控制台退出后由 main 调用）：
// 关闭顺序：前端窗口 → HTTP 服务 → 人格层资源 → WebSocket 集线器。
// 优雅关闭放在独立 goroutine 并设兜底超时：卡在 webview Run() 中的窗口
// （见 browser_client/desktop.go 文件头）可能使关闭无法及时返回，超时后交由 main 的 os.Exit(0) 收尾。
func GracefulShutdown(reason string) {
	LoggerGeneral.Info("CorePersonality", "%s，正在关闭...", reason)

	done := make(chan struct{})
	go func() {
		defer close(done)
		BrowserClient.CloseAllDesktopWindows()
		server.Shutdown(httpServer)
		crystal.Close()
		shutdownLunar()
		// 统一资源目录：关闭 SQLite 连接（内容仓库中的 blob 不动，句柄持久保留）
		if err := file.CloseArchiveDB(); err != nil {
			LoggerGeneral.Warn("CorePersonality", "关闭资源目录失败: %v", err)
		}
		if ws.Instance != nil {
			ws.Instance.CloseAll()
		}
	}()

	select {
	case <-done:
		LoggerGeneral.Info("CorePersonality", "已安全关闭")
	case <-time.After(shutdownTimeout):
		LoggerGeneral.Warn("CorePersonality", "优雅关闭超时（%s），强制退出", shutdownTimeout)
	}
}
