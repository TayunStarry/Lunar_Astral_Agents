package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"CorePersonality/engine/tts_router"
	"CorePersonality/personality/crystal"
	"CorePersonality/personality/lunar/bridging/napcat"
	"CorePersonality/personality/lunar/engine"
	"CorePersonality/server"
	"CorePersonality/transport/enginebus"
	"CorePersonality/transport/ws"
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
)

// Assemble 装配各子系统：传输层 → 人格层 → 引擎层 → HTTP 服务。
// 各层按依赖顺序初始化，跨层依赖（引擎总线发送函数、结构化消息桥接）在此注入，
// 避免包间反向依赖，亦避免各人格包互相 import。
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
	enginebus.Instance.Subscribe(engine.AnimationObserver{})
	enginebus.Instance.Subscribe(engine.TelemetryObserver{})
	LoggerGeneral.Info("CorePersonality", "引擎消息总线（反射 + 透传）已启动")

	// ==== 跨层注入：结构化广播 & 下行桥接 ====
	// 智能体下行走结构化广播（type/data），QQ 桥接挂在结构化旁路上（原始字节广播不触发）
	engine.PushMessageFunc = ws.Instance.BroadcastMessage
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
		return BrowserClient.OpenDesktopWindow(crystalWindowSpec())
	})
	crystal.SetFrontendReadyProbe(func() bool {
		return ws.Instance != nil && ws.Instance.ChannelBusCount(ws.DefaultBusChannel) > 0
	})

	// ==== TTS 闲置卸载守护：三引擎 5 分钟无合成调用即卸载模型占用 ====
	// 引擎本体在首次合成请求时惰性加载（engine/tts_router），本守护只负责闲置回收。
	ttsrouter.StartIdleUnloader()

	// ==== 入站消费：ltp9/* 调试信封交由 LTP9 引擎处理 ====
	ws.Instance.SubscribeInbound("ltp9", crystal.HandleInbound)
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

// OpenDesktopWindows 打开两个桌面前端窗口：月华消息终端（/）与琉璃桌面（/desktop）
func OpenDesktopWindows() {
	for _, spec := range []BrowserClient.DesktopWindowSpec{lunarWindowSpec(), crystalWindowSpec()} {
		if err := BrowserClient.OpenDesktopWindow(spec); err != nil {
			LoggerGeneral.Error("CorePersonality", "桌面窗口 [%s] 打开失败: %v", spec.ID, err)
		}
	}

	// 打开窗口后开始监视前端会话（退出判定，见函数注释）
	go watchFrontendSessions()
}

// watchFrontendSessions 监视前端 WebSocket 会话，作为「桌面页面是否已关闭」的可靠判据。
//
// 为何不能只依赖窗口关闭事件：实测 webview_go 在当前环境下，窗口被关闭后其
// `Run()` 可能不返回（窗口与渲染进程确已销毁，事件循环却不退出），
// 只按 Run() 计数将永远等不到「全部窗口关闭」，表现为「窗口全关而服务端不退」。
//
// 而前端页面与后端的 /ws 长连接是可靠可观测的：全部页面关闭后连接数必然归零。
// 为避免刷新/导航造成瞬时归零误判，需「曾有过连接（armed）」且连续多次采样为空才判定关闭。
func watchFrontendSessions() {
	armed := false
	quiet := 0
	for {
		time.Sleep(sessionSampleInterval)
		if ws.Instance == nil {
			continue
		}
		if count := ws.Instance.ClientCount(); count > 0 {
			armed = true
			quiet = 0
			continue
		}
		// 从未有过前端连接（如无窗口的运行模式）时不作判定
		if !armed {
			continue
		}
		quiet++
		if quiet >= sessionQuietSamples {
			LoggerGeneral.Info("CorePersonality", "全部前端页面已关闭（WebSocket 会话归零并持续 %d 次采样），触发退出", quiet)
			notifyShutdown()
			return
		}
	}
}

// WaitForShutdown 阻塞等待退出信号（系统信号或任一桌面窗口关闭）并优雅关闭
func WaitForShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		LoggerGeneral.Info("CorePersonality", "接收到中断信号，正在关闭...")
	case <-BrowserClient.DesktopWindowsClosed():
		LoggerGeneral.Info("CorePersonality", "检测到全部桌面窗口关闭，正在关闭...")
	case <-shutdownCh:
		LoggerGeneral.Info("CorePersonality", "检测到前端页面全部关闭，正在关闭...")
	}

	// 优雅关闭放在独立 goroutine 并设兜底超时：
	// 卡在 webview Run() 中的窗口（见 browser_client/desktop.go 文件头）可能使
	// Terminate / HTTP 关闭无法及时返回，超时后不再等待，交由 main 的 os.Exit(0) 收尾。
	done := make(chan struct{})
	go func() {
		defer close(done)
		// 关闭顺序：桌面窗口 → HTTP 服务 → 人格层资源 → WebSocket 集线器
		BrowserClient.CloseAllDesktopWindows()
		server.Shutdown(httpServer)
		crystal.Close()
		shutdownLunar()
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
