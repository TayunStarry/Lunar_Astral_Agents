package proxy

import (
	"bytes"
	"config"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"logger"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"browser"
)

//go:embed frontend/*
var frontendFS embed.FS

// ProxyServerPort 代理服务器HTTPS监听端口
const ProxyServerPort = 36369

// CORSAllowedOrigins 定义允许跨域访问的来源列表
var CORSAllowedOrigins = []string{
	fmt.Sprintf("http://localhost:%d", *config.BasicPort),
	fmt.Sprintf("https://localhost:%d", ProxyServerPort),
}

// 前端资源路径前缀
const frontendPathPrefix = "/proxy_ui"

// 请求映射和互斥锁
var requests = make(map[string]*any)
var serverMutex sync.RWMutex

// Run 启动代理服务器的完整流程：获取IP→启动HTTPS服务→嵌入前端资源→打开浏览器，返回服务器实例
func Run() *http.Server {
	logger.SetDevMode(*config.Developer)

	// 1. 获取本地IP地址
	ip, err := browser.GetLocalIP(nil)
	if err != nil {
		logger.Warn("ProxySvr", "获取本地IP失败: %v，使用localhost", err)
		ip = "localhost"
	}
	logger.Info("ProxySvr", "本地IP地址: %s", ip)

	// 2. 启动代理服务器
	server := StartProxyServer()
	if server == nil {
		logger.Error("ProxySvr", "代理服务器启动失败")
		return nil
	}

	// 3. 使用browser包的WebView功能打开前端界面
	serverURL := fmt.Sprintf("https://%s:%d/proxy_ui", ip, ProxyServerPort)
	logger.Info("ProxySvr", "代理服务地址: %s", serverURL)
	browser.OpenBrowser(serverURL)

	return server
}

// StartProxyServer 启动HTTPS代理服务器，监听36369端口
func StartProxyServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequest)

	// 生成自签名证书
	cert, err := generateSelfSignedCert()
	if err != nil {
		logger.Error("ProxySvr", "生成证书失败: %v", err)
		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", ProxyServerPort),
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	go func() {
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Error("ProxySvr", "代理服务器启动失败: %v", err)
		}
	}()

	logger.Info("ProxySvr", "代理服务器已启动 -> https://localhost:%d/", ProxyServerPort)
	logger.Info("ProxySvr", "前端界面 [GET] -> https://localhost:%d/proxy_ui", ProxyServerPort)
	logger.Info("ProxySvr", "健康检查 [GET] -> https://localhost:%d/health", ProxyServerPort)
	logger.Info("ProxySvr", "服务器信息 [GET] -> https://localhost:%d/api/server-info", ProxyServerPort)

	return server
}

// BuildTLSTerminationProxy 构建一个HTTPS终止代理服务器，接收外部HTTPS请求，将其解密后转发给内部的HTTP服务器
func BuildTLSTerminationProxy() *http.Server {
	logger.SetDevMode(*config.Developer)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealthCheck)
	mux.HandleFunc("/", handleReverseProxy)
	serverAddr := fmt.Sprintf(":%d", *config.ProxyPort)
	logger.Info("ProxySvr", "代理请求 [POST] -> https://localhost:%v/", *config.ProxyPort)
	logger.Info("ProxySvr", "健康检查 [GET] -> https://localhost:%v/health", *config.ProxyPort)

	// 生成自签名证书
	cert, err := generateSelfSignedCert()
	if err != nil {
		logger.Error("ProxySvr", "生成证书失败: %v", err)
		return nil
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server := &http.Server{
		Addr:      serverAddr,
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	go func() {
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			logger.Error("ProxySvr", "服务器启动失败: %v", err)
		}
	}()

	return server
}

// handleRequest 统一请求处理器：/proxy_ui路径由本地处理，其余全部代理转发
func handleRequest(w http.ResponseWriter, r *http.Request) {
	logRequest(r)
	setCORSHeaders(w, r)

	// 处理预检请求
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 前端资源请求由本地处理
	if strings.HasPrefix(r.URL.Path, frontendPathPrefix) {
		serveFrontend(w, r)
		return
	}

	// 健康检查端点
	if r.URL.Path == "/health" {
		handleHealthCheck(w, r)
		return
	}

	// 服务器信息API
	if r.URL.Path == "/api/server-info" {
		handleServerInfo(w, r)
		return
	}

	// WebSocket连接代理
	if r.Header.Get("Upgrade") == "websocket" {
		handleWebSocketProxy(w, r)
		return
	}

	// 其余请求全部代理转发至目标HTTP服务器
	handleReverseProxy(w, r)
}

// serveFrontend 提供嵌入式前端资源，embed路径与URL路径直接对应
func serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// /proxy_ui 或 /proxy_ui/ 重定向到 /proxy_ui/index.html
	if path == frontendPathPrefix || path == frontendPathPrefix+"/" {
		path = frontendPathPrefix + "/index.html"
	}

	// embed FS路径: frontend/proxy_ui/xxx -> URL: /proxy_ui/xxx
	content, err := frontendFS.ReadFile("frontend" + path)
	if err != nil {
		http.Error(w, "资源未找到", http.StatusNotFound)
		return
	}

	contentType := "text/plain; charset=utf-8"
	switch {
	case strings.HasSuffix(path, ".html"):
		contentType = "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		contentType = "application/javascript; charset=utf-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(content)
}

// handleServerInfo 返回服务器信息API
func handleServerInfo(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	ip, _ := browser.GetLocalIP(nil)
	info := map[string]any{
		"ip":         ip,
		"port":       ProxyServerPort,
		"url":        fmt.Sprintf("https://%s:%d", ip, ProxyServerPort),
		"basic_port": *config.BasicPort,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(info); err != nil {
		http.Error(w, "编码响应失败", http.StatusInternalServerError)
	}
}

// handleHealthCheck 用于监控系统状态和负载情况
func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serverMutex.RLock()
	pendingRequests := len(requests)
	serverMutex.RUnlock()

	healthStatus := map[string]any{
		"status":           "healthy",
		"timestamp":        time.Now(),
		"pending_requests": pendingRequests,
		"port":             ProxyServerPort,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(healthStatus); err != nil {
		http.Error(w, "编码响应失败", http.StatusInternalServerError)
		return
	}
}

// setCORSHeaders 设置跨域资源共享（CORS）响应头，支持常见HTTP方法
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowed := slices.Contains(CORSAllowedOrigins, origin)
	if allowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// handleReverseProxy 处理反向代理请求，将HTTPS请求解密后转发给内部的HTTP服务器
func handleReverseProxy(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		handleWebSocketProxy(w, r)
		return
	}

	targetURL := fmt.Sprintf("http://localhost:%d", *config.BasicPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "目标URL解析失败", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		originalURL := req.URL.String()
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Proto", "https")
		_, port, splitErr := net.SplitHostPort(r.Host)
		if splitErr != nil || port == "" {
			port = "443"
		}
		req.Header.Set("X-Forwarded-Port", port)
		logger.Info("ProxySvr", "转发请求: %s %s -> %s%s", req.Method, originalURL, targetURL, req.URL.Path)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("ProxySvr", "代理错误: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		errorMsg := fmt.Sprintf(`{"error": "无法连接到后端服务器", "message": "%s"}`, err.Error())
		w.Write([]byte(errorMsg))
	}
	proxy.ServeHTTP(w, r)
}

// handleWebSocketProxy 处理WebSocket代理，将WSS连接转发到内部WS服务
func handleWebSocketProxy(w http.ResponseWriter, r *http.Request) {
	targetAddr := fmt.Sprintf("localhost:%d", *config.BasicPort)

	backendConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		logger.Error("ProxySvr", "WebSocket连接后端失败: %v", err)
		http.Error(w, "无法连接到后端WebSocket服务", http.StatusBadGateway)
		return
	}
	defer backendConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		logger.Error("ProxySvr", "不支持Hijack接口")
		http.Error(w, "不支持的协议升级", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		logger.Error("ProxySvr", "Hijack失败: %v", err)
		http.Error(w, "连接劫持失败", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	var reqBuffer bytes.Buffer
	r.Write(&reqBuffer)
	backendConn.Write(reqBuffer.Bytes())

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(backendConn, clientBuf)
		io.Copy(backendConn, clientConn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, backendConn)
	}()

	wg.Wait()
	logger.Info("ProxySvr", "WebSocket连接关闭")
}

// logRequest 记录请求日志
func logRequest(r *http.Request) {
	logger.Info("ProxySvr", "[%s] %s %s", r.Method, r.RemoteAddr, r.URL.String())
}

// ReadCertFile 读取证书文件（兼容性保留）
func ReadCertFile() ([]byte, error) {
	if storedCertPEM == nil {
		return nil, fmt.Errorf("证书尚未生成")
	}
	return storedCertPEM, nil
}

// ReadKeyFile 读取私钥文件（兼容性保留）
func ReadKeyFile() ([]byte, error) {
	if storedKeyPEM == nil {
		return nil, fmt.Errorf("私钥尚未生成")
	}
	return storedKeyPEM, nil
}
