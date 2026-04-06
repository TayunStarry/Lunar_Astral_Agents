package server

import (
	"Lunar-Astral-Agents/model"
	"Lunar-Astral-Agents/parameter"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"sync"
	"time"
)

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{fmt.Sprintf("http://localhost:%d", *parameter.BasicPort)}

// 请求映射，键为请求ID，值为请求上下文
var requests = make(map[string]*model.RequestContext)

// 互斥锁，用于保护请求映射的并发访问
var serverMutex sync.RWMutex

// BuildTLSTerminationProxy 构建一个HTTPS终止代理服务器，接收外部HTTPS请求，将其解密后转发给内部的HTTP服务器
func BuildTLSTerminationProxy() *http.Server {
	// 创建独立的ServeMux实例
	mux := http.NewServeMux()
	// 注册健康检查处理器
	mux.HandleFunc("/health", handleHealthCheck)
	// 注册反向代理处理器，将所有请求转发到HTTP后端
	mux.HandleFunc("/", handleReverseProxy)
	// 构建服务器地址
	serverAddr := fmt.Sprintf(":%d", *parameter.ProxyPort)
	// 打印代理服务访问地址
	log.Printf("Lunar模块[TLS代理] : 代理请求 [POST] -> https://localhost:%v/", *parameter.ProxyPort)
	log.Printf("Lunar模块[TLS代理] : 健康检查 [GET] -> https://localhost:%v/health", *parameter.ProxyPort)
	// 创建服务器实例
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}
	// 启动HTTPS服务器（在独立goroutine中运行）
	go func() {
		if err := http.ListenAndServeTLS(serverAddr, *parameter.CertFile, *parameter.KeyFile, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("Lunar模块[TLS代理][ERROR] -> 服务器启动失败: %v\n", err)
		}
	}()
	// 返回服务器实例
	return server
}

// handleHealthCheck 用于监控系统状态和负载情况
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// 添加CORS头，允许跨域访问
	setCORSHeaders(w, r)
	// 如果是预检请求（OPTIONS），直接返回200 OK
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// 只允许GET方法，否则返回405 Method Not Allowed
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 使用读锁获取当前挂起的请求数量
	serverMutex.RLock()
	pendingRequests := len(requests)
	serverMutex.RUnlock()
	// 构造健康检查响应数据
	healthStatus := map[string]any{
		"status":           "healthy",         // 服务状态：健康
		"timestamp":        time.Now(),        // 当前时间戳
		"pending_requests": pendingRequests,   // 当前挂起的请求数
		"port":             *parameter.ProxyPort, // 服务监听端口
	}
	// 设置响应头，指定返回JSON格式
	w.Header().Set("Content-Type", "application/json")
	// 将健康状态编码为JSON并写入响应体
	if err := json.NewEncoder(w).Encode(healthStatus); err != nil {
		// 编码失败，返回500内部服务器错误
		http.Error(w, "编码响应失败", http.StatusInternalServerError)
		return
	}
}

// setCORSHeaders 设置跨域资源共享（CORS）响应头，允许前端应用从不同的源访问API
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	// 从请求头中获取来源（Origin）
	origin := r.Header.Get("Origin")
	// 检查该来源是否在服务器允许的白名单中
	allowed := slices.Contains(CORSAllowedOrigins, origin)
	// 如果来源被允许，则将其写回响应头，允许该来源跨域访问
	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		// 否则使用白名单中的第一个来源作为默认值，避免暴露空值
		w.Header().Set("Access-Control-Allow-Origin", CORSAllowedOrigins[0])
	}
	// 设置允许的HTTP方法：POST、GET、OPTIONS
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	// 设置允许的请求头：Content-Type、Authorization
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	// 设置预检请求（OPTIONS）的缓存时间，单位为秒，86400秒即24小时
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// handleReverseProxy 处理反向代理请求，将HTTPS请求解密后转发给内部的HTTP服务器，并将响应返回给客户端， 实现TLS终止（TLS Termination）模式，让内部服务无需处理HTTPS
func handleReverseProxy(w http.ResponseWriter, r *http.Request) {
	// 添加CORS头
	setCORSHeaders(w, r)
	// 处理预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	// 目标服务器地址 - 内部HTTP服务器的URL
	targetURL := fmt.Sprintf("http://localhost:%d", *parameter.BasicPort)
	// 解析目标URL - 将字符串URL转换为url.URL对象
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "目标URL解析失败", http.StatusInternalServerError)
		return
	}
	// 创建反向代理 - 使用标准库创建单主机反向代理
	proxy := httputil.NewSingleHostReverseProxy(target)
	// 自定义请求处理器 - 配置请求转发规则
	proxy.Director = func(req *http.Request) {
		// 保存原始URL用于日志 - 记录客户端请求的原始URL
		originalURL := req.URL.String()
		// 设置协议为后端服务器的协议（HTTP）
		req.URL.Scheme = target.Scheme
		// 设置主机为后端服务器的主机
		req.URL.Host = target.Host
		// 设置Host头为后端服务器的主机
		req.Host = target.Host
		// 指示原始请求使用HTTPS（用于后端服务识别原始协议）
		req.Header.Set("X-Forwarded-Proto", "https")
		// 指示原始请求的端口
		req.Header.Set("X-Forwarded-Port", fmt.Sprintf("%d", *parameter.ProxyPort))
		// 开发模式日志 - 记录请求转发详情
		if *parameter.DevMode {
			log.Printf("[TLS代理] 转发请求: %s %s -> %s%s", req.Method, originalURL, targetURL, req.URL.Path)
		}
	}
	// 自定义错误处理器 - 处理代理过程中的错误
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// 记录错误日志 - 输出详细的错误信息
		log.Printf("[TLS代理] 代理错误: %v", err)
		// 设置错误响应头 - 返回JSON格式的错误信息
		w.Header().Set("Content-Type", "application/json")
		// 使用502状态码表示网关错误
		w.WriteHeader(http.StatusBadGateway)
		// 写入错误响应体 - 提供友好的错误信息
		errorMsg := fmt.Sprintf(`{"error": "无法连接到后端服务器", "message": "%s"}`, err.Error())
		w.Write([]byte(errorMsg))
	}
	// 执行代理请求 - 调用反向代理处理请求
	proxy.ServeHTTP(w, r)
}
