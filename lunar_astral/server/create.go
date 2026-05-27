package server

import (
	"browser"
	"config"
	"fmt"
	"logger"
	"net"
	"net/http"
)

// StartServerListener 启动服务器监听循环
func StartServerListener(server *http.Server) {
	// 为服务器添加 CORS 中间件
	server.Handler = CORSMiddleware(httpMux)
	//拼接服务器监听地址
	addr := fmt.Sprintf(":%d", *config.BasicPort)
	// 监听指定端口
	listener, err := net.Listen("tcp", addr)
	// 处理监听失败的情况
	if err != nil {
		logger.Fatal("LunarCore", "端口 %d 已被占用, 无法启动服务器, 请检查端口号配置", *config.BasicPort)
	}
	// 关闭监听器
	defer listener.Close()
	// 启动客户端加载任务
	go startClientLoading()
	// 启动服务器监听循环
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		logger.Fatal("LunarCore", "服务器运行失败: %v", err)
	}
}

// CORSMiddleware CORS 中间件
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 设置 CORS 相关头信息，允许所有来源访问，支持多种 HTTP 方法和请求头
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理 OPTIONS 请求，直接返回 200 状态码
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 调用下一个处理器
		next.ServeHTTP(w, r)
	})
}

// startClientLoading 启动客户端加载任务
func startClientLoading() {
	// 获取本地 IP 地址
	ip, err := browser.GetLocalIP([]string{})
	// 处理获取 IP 地址失败的情况
	if err != nil {
		logger.Error("LunarCore", "%v", err)
		return
	}
	// 构建客户端访问的 URL
	//clientUrl := fmt.Sprintf("http://localhost:%d", *config.BasicPort)
	// 构建内部接口的 URL
	internalURL := fmt.Sprintf("http://%s:%d", ip, *config.BasicPort)
	// 打开浏览器访问内部接口
	browser.OpenBrowser(internalURL)
}
