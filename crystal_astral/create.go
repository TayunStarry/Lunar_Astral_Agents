package main

import (
	"LunarSubsystem/BrowserClient"
	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	image "LunarSubsystem/MultimodalAnalysis/server"
	"LunarSubsystem/LoggerGeneral"
	"CrystalAstral/agent/YaraLTP"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// shouldProxy 判断是否需要代理路径
func shouldProxy(path string) bool {
	for _, prefix := range proxyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// copyBuffer 复制缓冲区内容到目标入参
func copyBuffer(dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				er = nil
			}
			return written, er
		}
	}
}

// reloadPageParameters 重新加载页面参数
func reloadPageParameters() {
	*GeneralConfig.WebViewTitle = "[星月智能] : 钛宇-琉璃"
	*GeneralConfig.WebViewWidth = 1500
	*GeneralConfig.WebViewHeight = 1050
	// 窗口图标（相对路径按「可执行文件目录→当前工作目录」解析，exe 输出在仓库根目录）
	*GeneralConfig.WebViewIconPath = filepath.Join("crystal_astral", "icon.ico")
}

// initMemoryDatabase 自动初始化记忆库实例与默认集合（v2 标签向量架构）
// 模型配置（嵌入模型、多模态模型、API 地址）从 config 模块（lunar_config.json）读取
// 失败仅打印警告，不阻断服务启动，用户仍可通过记忆库面板手动初始化
func initMemoryDatabase() {
	// 第一步：实例初始化（嵌入服务 + LLM 标签生成服务，模型配置从 config 模块读取）
	if err := file.MemoryInitInstance(); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "记忆库实例初始化失败: %v (可手动通过记忆库面板初始化)", err)
		return
	}
	LoggerGeneral.Info("CrystalAstral", "记忆库实例初始化完成 (模型配置从 lunar_config.json 读取)")

	// 第二步：创建/打开默认集合（探针文本嵌入定维度，含网络请求，加超时保护）
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := file.CollectionInit(ctx, defaultMemoryCollection, defaultMemoryModelName, file.CollectionTypeText); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "集合 [%s] 创建失败: %v (可手动通过记忆库面板初始化)", defaultMemoryCollection, err)
		return
	}
	LoggerGeneral.Info("CrystalAstral", "集合 [%s] 创建成功, 模型: %s", defaultMemoryCollection, defaultMemoryModelName)
}

// StartServer 启动服务器
func StartServer(port int, root http.FileSystem, name string) error {
	// 默认仅绑定 127.0.0.1 回环地址（浏览器/月华均经本机访问）：
	// /ws 无鉴权且承载 LTP9 调试面（文件/数据库/指令/网络探针），对局域网开放等于开放控制面。
	// 跨机部署时经 lunar_config.json 的 server.allow_lan 显式放开。
	bindHost := "127.0.0.1"
	if *GeneralConfig.AllowLAN {
		bindHost = ""
	}
	serverAddr := fmt.Sprintf("%s:%d", bindHost, port)
	// 对外展示/浏览器访问用的 URL：绑全部网卡（allow_lan）时用 localhost，
	// 回环绑定时用 127.0.0.1（不可与 serverAddr 直接拼接，那会拼出 localhost127.0.0.1）
	urlHost := bindHost
	if urlHost == "" {
		urlHost = "localhost"
	}
	serviceURL := fmt.Sprintf("http://%s:%d", urlHost, port)
	ln, err := net.Listen("tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("监听端口 %s 失败(端口可能被占用): %w", serverAddr, err)
	}
	// 初始化知识库（SQLite）
	if err := file.InitKnowledgeDB(*GeneralConfig.KnowledgeDBPath); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "知识库初始化失败: %v (不影响服务启动)", err)
	}
	// 初始化记忆库存储目录（仅准备本地存储结构，不产生网络请求）
	file.InitMemoryDB(*GeneralConfig.MemoryDBDir)
	// 自动初始化记忆库实例与默认集合（与 lunar_astral 的 JS agent 行为对齐）
	// 模型配置从 lunar_config.json 的 agent 配置组读取
	initMemoryDatabase()
	// 启动图像生成任务处理器
	image.StartTaskProcessor()
	httpMux := http.NewServeMux()

	// 初始化工作室 WebSocket 集线器（无差别广播：收到任意消息转发给所有 /ws 客户端，客户端自行过滤）
	StudioHubInstance = NewStudioHub()
	go StudioHubInstance.Run()
	httpMux.HandleFunc("/ws", StudioHubInstance.HandleWebSocket)

	// LTP3（YaraFlow）引擎：注入出站发送函数 + 初始化并加载插件 + 启动入站信封消费
	YaraLTP.SetSend(func(data []byte) {
		if StudioHubInstance != nil {
			select {
			case StudioHubInstance.Broadcast <- data:
			default:
			}
		}
	})
	if err := YaraLTP.Init(); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "LTP3 引擎初始化失败: %v (不影响服务启动)", err)
	}
	go func() {
		for data := range StudioHubInstance.Inbound {
			// LTP9 调试信封先消费，其余交给 LTP3（YaraFlow）引擎
			if ltp9HandleInbound(data) {
				continue
			}
			YaraLTP.HandleIn(data)
		}
	}()
	// LTP9 引擎：接入项目记忆库/SQLite，并注入前端智能体（Mini-LTP/Node-LTP）真实通道
	if err := BridgeLTP9(); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "LTP9 引擎初始化失败: %v (不影响服务启动)", err)
	}

	fsHandler := http.FileServer(root)
	for _, endpoint := range SystemEndpoints {
		httpMux.HandleFunc(endpoint.Path, endpoint.Handler)
	}
	proxy := getProxyHandler()
	proxyHandler := &proxyAwareHandler{
		fs:          fsHandler,
		proxy:       proxy,
		shouldProxy: shouldProxy,
	}
	httpMux.Handle("/", proxyHandler)

	server := &http.Server{
		Addr:    serverAddr,
		Handler: httpMux,
	}

	LoggerGeneral.Info("CrystalAstral", "%s 正运行在 %s", name, serviceURL)
	reloadPageParameters()
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer)
	// 使用带「返回主页面」悬浮按钮的 webview 打开：
	// 应用被外部链接 _self 跳转走后，仍能通过悬浮按钮回到琉璃主页面
	go BrowserClient.OpenBrowserWithReturnButton(serviceURL, serviceURL+"/")

	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			LoggerGeneral.Error("CrystalAstral", "%s 运行失败: %v", name, err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		LoggerGeneral.Info("CrystalAstral", "%s 接收到中断信号，正在关闭...", name)
	case <-BrowserClient.WebViewClosed():
		LoggerGeneral.Info("CrystalAstral", "%s 检测到 WebView 关闭，正在关闭...", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		LoggerGeneral.Error("CrystalAstral", "%s 关闭失败: %v", name, err)
	}

	BrowserClient.CloseWebView()
	YaraLTP.Close()
	LoggerGeneral.Info("CrystalAstral", "%s 已成功关闭", name)

	return nil
}

// ServeHTTP 处理 HTTP 请求
// 根据路径判断是否需要通过代理转发
func (h *proxyAwareHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if h.shouldProxy(path) {
		h.proxy.ServeHTTP(w, r)
	} else {
		h.fs.ServeHTTP(w, r)
	}
}

// Gethierarchy 返回嵌入的文件系统
func Gethierarchy() http.FileSystem {
	// 创建一个子文件系统，只包含assets目录下的内容
	subFS, err := fs.Sub(EmbeddedFiles, "assets")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
