package server

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
)

// StartServerListener 启动服务器监听循环
func StartServerListener(server *http.Server) {
	// 为服务器添加 CORS 中间件
	server.Handler = CORSMiddleware(httpMux)
	//拼接服务器监听地址
	addr := fmt.Sprintf(":%d", *GeneralConfig.BasicPort)
	// 监听指定端口
	listener, err := net.Listen("tcp", addr)
	// 处理监听失败的情况
	if err != nil {
		LoggerGeneral.Fatal("LunarCore", "端口 %d 已被占用, 无法启动服务器, 请检查端口号配置", *GeneralConfig.BasicPort)
	}
	// 关闭监听器
	defer listener.Close()
	// 启动客户端加载任务
	go startClientLoading()
	// 启动服务器监听循环
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		LoggerGeneral.Fatal("LunarCore", "服务器运行失败: %v", err)
	}
}

// CORSMiddleware CORS 中间件
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 处理 OPTIONS 预检请求，直接返回 200
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.WriteHeader(http.StatusOK)
			return
		}

		// 调用下一个处理器
		next.ServeHTTP(w, r)

		// 在处理器返回后覆盖 CORS 头，避免与上游（如代理）重复
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	})
}

// startClientLoading 启动客户端加载任务
func startClientLoading() {
	// 优先使用 127.0.0.1 访问本地服务，避免防火墙拦截和跨网段问题
	clientURL := fmt.Sprintf("http://127.0.0.1:%d", *GeneralConfig.BasicPort)
	*GeneralConfig.WebViewWidth = 1080
	*GeneralConfig.WebViewHeight = 810
	*GeneralConfig.WebViewTitle = "[星月智能] : 钛宇-月华"
	// 窗口图标（相对路径按「可执行文件目录→当前工作目录」解析，exe 输出在仓库根目录）
	*GeneralConfig.WebViewIconPath = filepath.Join("lunar_astral", "icon.ico")
	// 打开浏览器访问
	BrowserClient.OpenBrowser(clientURL)
}