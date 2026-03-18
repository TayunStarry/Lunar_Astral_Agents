package server

// 导入必要的包
import (
	config "Lunar-Astral-Agents/parameter"    // 引入配置模块，用于获取模型路径等配置
	utils "Lunar-Astral-Agents/utils"         // 工具函数包
	browser "Lunar-Astral-Agents/browser"  // 导入浏览器模块（如打开浏览器）
	websocket "Lunar-Astral-Agents/websocket" // WebSocket相关包
	"fmt"                                     // 用于格式化输入输出
	"log"                                     // 用于日志记录
	"net/http"                                // 用于构建HTTP服务器
	"strings"                                 // 用于字符串操作
	"time"                                    // 用于时间相关操作
)

// 全局变量
var websocketServer *http.Server

// StartServer 启动HTTP服务器
func StartServer() *http.Server {
	// 创建一个新的 HTTP 服务器实例
	server := &http.Server{}
	// 启动服务器监听
	go startServerListener(server)
	return server
}

// startServerListener 启动服务器监听循环
func startServerListener(server *http.Server) {
	// 打印启动服务器的日志信息
	log.Printf("%s", strings.Repeat("-=", 28))
	// 定义最大尝试次数
	const maxAttempts = 10
	// 尝试启动服务器
	if !attemptServerStart(server, maxAttempts) {
		log.Fatalf("Lunar模块[ERROR] -> 无可用端口")
	}
}

// attemptServerStart 尝试启动服务器，最多尝试指定次数
func attemptServerStart(server *http.Server, maxAttempts int) bool {
	for i := 0; i < maxAttempts; i++ {
		if tryStartServerOnPort(server) {
			return true
		}
		*config.BasicPort++
	}
	return false
}

// tryStartServerOnPort 尝试在指定端口上启动服务器
func tryStartServerOnPort(server *http.Server) bool {
	// 服务器成功启动后的初始化工作
	initializeServerComponents(server)
	// 配置服务器监听地址
	addr := fmt.Sprintf(":%d", *config.BasicPort)
	// 启动HTTPS服务器 - 使用TLS证书提供安全访问
	if err := http.ListenAndServeTLS(addr, *config.CertFile, *config.KeyFile, server.Handler); err != nil && err != http.ErrServerClosed {
		log.Printf("Lunar模块[ERROR] -> %v", err)
		return false
	}
	// 返回成功启动服务器
	return true
}

// initializeServerComponents 初始化服务器组件
func initializeServerComponents(server *http.Server) {
	// 为服务器添加 CORS 中间件
	server.Handler = utils.CORSMiddleware(httpMux)
	// 启动客户端加载任务
	go startClientLoading()
	// 构建模拟服务器并保存实例
	websocketServer = websocket.BuildSimulatedServer()
}

// startClientLoading 启动客户端加载任务
func startClientLoading() {
	// 获取本地 IP 地址
	ip, err := utils.GetLocalIP([]string{})
	// 处理获取 IP 地址失败的情况
	if err != nil {
		log.Printf("Lunar模块[ERROR] -> %v\n", err)
		return
	}
	// 构建客户端访问的 URL
	//clientUrl := fmt.Sprintf("https://localhost:%d", *config.BasicPort)
	// 构建内部接口的 URL
	internalURL := fmt.Sprintf("https://%s:%d", ip, *config.BasicPort)
	// 检查是否使用 webview
	if *config.UseWebView {
		// 使用 webview 内嵌浏览器
		if browser.IsWebViewSupported() {
			go startWebViewBrowser(internalURL)
		} else {
			log.Printf("Webview[ERROR] -> 当前系统不支持 webview，回退到系统浏览器")
			browser.OpenBrowser(internalURL)
		}
	} else {
		// 检查是否非开发模式，如果不是开发模式，则自动打开浏览器访问服务器
		if !*config.DevMode {
			browser.OpenBrowser(internalURL)
		}
	}
	// 打印服务器端口
	PrintServerPort(internalURL)
}

// startWebViewBrowser 启动 webview 浏览器
func startWebViewBrowser(url string) {
	// 等待服务器启动完成
	time.Sleep(1 * time.Second)

	// 创建 webview 配置
	webviewConfig := browser.WebViewConfig{
		Title:     *config.WebViewTitle,
		Width:     *config.WebViewWidth,
		Height:    *config.WebViewHeight,
		Resizable: *config.WebViewResizable,
		Debug:     *config.WebViewDebug,
	}

	// 创建 webview 实例
	w := browser.CreateWebView(webviewConfig)
	if w == nil {
		log.Printf("Webview[ERROR] -> 无法创建 webview 实例")
		return
	}

	// 导航到指定 URL
	browser.NavigateWebView(url)

	// 运行 webview（阻塞）
	browser.RunWebView()
}

// CloseWebSocketServer 关闭WebSocket服务器
func CloseWebSocketServer() {
	if websocketServer != nil {
		log.Printf("Lunar模块[WebSocket] -> 关闭服务器")
		websocketServer.Close()
		websocketServer = nil
	}
}

// PrintServerPort 打印服务器端口
func PrintServerPort(internalURL string) {
	// 打印分割线
	log.Printf("%s", strings.Repeat("-=", 28))
	// 遍历所有系统端点并打印
	for _, endpoint := range SystemEndpoints {
		log.Printf("Lunar模块 : %s [%s]	-> %v%s", endpoint.Description, endpoint.Method, internalURL, endpoint.Path)
	}
	// 打印前端文件访问路径
	log.Printf("Lunar模块 : 前端文件 [GET]	-> %v/", internalURL)
}
