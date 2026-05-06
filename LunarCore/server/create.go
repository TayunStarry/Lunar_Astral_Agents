package server

import (
	"browser"
	"config"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// StartServerListener 启动服务器监听循环
func StartServerListener(server *http.Server) {
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
	for range maxAttempts {
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
	// 启动HTTP服务器
	if err := http.ListenAndServe(addr, server.Handler); err != nil && err != http.ErrServerClosed {
		log.Printf("Lunar模块[ERROR] -> %v", err)
		return false
	}
	// 返回成功启动服务器
	return true
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

// initializeServerComponents 初始化服务器组件
func initializeServerComponents(server *http.Server) {
	// 为服务器添加 CORS 中间件
	server.Handler = CORSMiddleware(httpMux)
	// 启动客户端加载任务
	go startClientLoading()
}

// PrintServerPort 打印服务器端口
func PrintServerPort(internalURL string) {
	// 打印分割线
	log.Printf("%s", strings.Repeat("-=", 28))
	// 检查是否为开发模式
	if *config.Developer == false {
		// 遍历所有系统端点并打印
		for _, endpoint := range SystemEndpoints {
			log.Printf("Lunar模块 : %s [%s]	-> %v%s", endpoint.Description, endpoint.Method, internalURL, endpoint.Path)
		}
		// 打印前端文件访问路径
		log.Printf("Lunar模块 : 前端文件 [GET]	-> %v/", internalURL)
		log.Printf("Lunar模块 : 消息推送 [WebSocket]	-> %v/ws", internalURL)
	}
}

// startClientLoading 启动客户端加载任务
func startClientLoading() {
	// 获取本地 IP 地址
	ip, err := browser.GetLocalIP([]string{})
	// 处理获取 IP 地址失败的情况
	if err != nil {
		log.Printf("Lunar模块[ERROR] -> %v\n", err)
		return
	}
	// 构建客户端访问的 URL
	//clientUrl := fmt.Sprintf("http://localhost:%d", *config.BasicPort)
	// 构建内部接口的 URL
	internalURL := fmt.Sprintf("http://%s:%d", ip, *config.BasicPort)
	// 打开浏览器访问内部接口
	browser.OpenBrowser(internalURL)
	// 打印服务器端口
	PrintServerPort(internalURL)
}
