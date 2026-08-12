package server

import (
	"LunarAstral/adapters"
	"LunarAstral/bridging/napcat"
	"LunarAstral/hierarchy"
	"LunarAstral/model/llama"
	"LunarAstral/server/handlers"
	"LunarAstral/websocket"
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	image "LunarSubsystem/ImageProcessor/server"
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/Qwen3-TTS/module"
	"context"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// InitializeServer 初始化服务器配置和组件
func InitializeServer() {
	// 设置日志开发模式
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer, "local_data/documents/debug")
	// 设置MIME类型映射
	for ext, mimeType := range GeneralConfig.MimeMap {
		mime.AddExtensionType(ext, mimeType)
	}
	// 创建本地目录
	if err := os.MkdirAll(*GeneralConfig.LocalDir, 0755); err != nil {
		LoggerGeneral.Fatal("LunarCore", "%v", err)
	}
	// 注册HTTP处理器
	registerHandlers()
	// 启动llama.cpp代理服务器
	llama.Init()
	// 初始化TTS语音合成引擎
	initTTSEngine()
	// 注册WebSocket处理器
	websocket.SetupWebSocketHandler(httpMux)
	// 注册音乐渲染回调（FluidSynth + SoundFont）
	adapters.MusicRenderFunc = handlers.RenderMusicInternal
	// 运行智能体上下文
	adapters.RunAgentContext()
	// 初始化桥接适配器
	initBridgeAdapter()
}

// registerHandlers 注册所有HTTP请求处理器
func registerHandlers() {
	// 创建独立的ServeMux实例
	httpMux = http.NewServeMux()
	// 处理根路径请求
	var fileServer http.Handler
	if *GeneralConfig.Developer {
		fileServer = http.FileServer(http.Dir("./lunar_astral/hierarchy/assets/client"))
		LoggerGeneral.Info("LunarCore", "使用开发模式，直接读取文件系统")
	} else {
		fileServer = http.FileServer(hierarchy.Gethierarchy())
	}
	httpMux.Handle("/", http.StripPrefix("/", fileServer))
	// 启动扩散生成任务协处理器
	image.StartTaskProcessor()
	// 注册所有系统端点路径的处理函数
	for _, endpoint := range SystemEndpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
	}
}

// SetupSignalHandling 设置系统信号处理
func SetupSignalHandling() chan os.Signal {
	// 创建一个用于接收系统信号的通道，缓冲区大小为 1
	quit := make(chan os.Signal, 1)
	// 监听 SIGINT 和 SIGTERM 信号，当接收到这些信号时，将信号发送到 quit 通道
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	return quit
}

// WaitForShutdown 等待关闭信号（系统信号或 WebView 关闭）并优雅关闭服务器
func WaitForShutdown(quit chan os.Signal, server *http.Server) {
	// 阻塞等待系统信号或 WebView 关闭信号，当接收到任一信号时继续执行后续代码
	select {
	case <-quit:
		LoggerGeneral.Info("LunarCore", "接收到中断信号，正在关闭...")
	case <-BrowserClient.WebViewClosed():
		LoggerGeneral.Info("LunarCore", "检测到 WebView 关闭，正在关闭...")
	}
	// 执行服务器关闭流程
	shutdownServer(server)
}

// initTTSEngine 初始化TTS语音合成引擎
func initTTSEngine() {
	modelDir := *GeneralConfig.LocalDir + "/models/Qwen3-TTS"
	refAudio := *GeneralConfig.LocalDir + "/audios/lunar-template.wav"
	module.InitTTSEngine(modelDir, refAudio)
}

// initBridgeAdapter 初始化桥接适配器
func initBridgeAdapter() {
	// 构建配置文件路径
	exePath, err := os.Executable()
	if err != nil {
		LoggerGeneral.Error("LunarCore", "获取可执行文件路径失败: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	configPath := filepath.Join(exeDir, *GeneralConfig.LocalDir, "lunar_config.json")

	// 加载桥接配置
	if err := napcat.LoadBridgingConfig(configPath); err != nil {
		LoggerGeneral.Error("LunarCore", "加载桥接配置失败: %v", err)
		return
	}

	// 注册桥接器消息回调
	napcat.SendMessageToAgent = func(messages []map[string]interface{}) {
		// 将QQ群消息（OpenAI格式）推送到智能体上下文
		for _, msg := range messages {
			adapters.UnreadContext = append(adapters.UnreadContext, adapters.PostMessage{
				Role:    "user",
				Content: msg["content"],
			})
		}
	}

	// 启动桥接器定时扫描
	napcat.StartBridgeScanner()
}

// shutdownServer 优雅关闭服务器
func shutdownServer(server *http.Server) {
	// 打印服务器正在关闭的信息
	LoggerGeneral.Info("LunarCore", "正在关闭...")
	// 创建一个带有 5 秒超时的上下文，用于控制服务器关闭的时间
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 确保在函数结束时取消上下文，释放资源
	defer cancel()
	// 关闭JavaScript运行时
	adapters.CloseAgentContext()
	// 关闭桥接适配器
	napcat.StopBridge()
	// 关闭llama.cpp服务器
	llama.Close()
	// 关闭WebSocket服务器
	websocket.CloseWebSocketServer()
	// 关闭WebView窗口（若未关闭则主动关闭）
	BrowserClient.CloseWebView()
	// 优雅地关闭服务器，等待所有活跃连接处理完成或超时
	if err := server.Shutdown(ctx); err != nil {
		// 如果关闭服务器时出错，打印错误信息并终止程序
		LoggerGeneral.Fatal("LunarCore", "%v", err)
	}
	// 打印服务器已安全关闭的信息
	LoggerGeneral.Info("LunarCore", "已安全关闭")
}
