package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"LunarSubsystem/general_config"
)

// ==== HTTPS 代理服务器 ====

// RunHTTPSProxy 启动 HTTPS 代理服务器，在终端中显示访问链接
// 将外部 HTTPS 请求解密后转发给内部 HTTP 后端服务
func RunHTTPSProxy() {
	scanner := bufio.NewScanner(os.Stdin)

	// 1. 获取后端端口
	fmt.Printf("请输入后端 HTTP 服务端口 [默认 %d]: ", *config.BasicPort)
	backendPort := *config.BasicPort
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			if v, err := strconv.Atoi(input); err == nil && v > 0 && v < 65536 {
				backendPort = v
			} else {
				fmt.Printf("  输入无效，使用默认端口: %d\n", *config.BasicPort)
			}
		}
	}

	// 2. 获取代理端口
	fmt.Printf("请输入代理 HTTPS 监听端口 [默认 %d]: ", *config.ProxyPort)
	proxyPort := *config.ProxyPort
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			if v, err := strconv.Atoi(input); err == nil && v > 0 && v < 65536 {
				proxyPort = v
			} else {
				fmt.Printf("  输入无效，使用默认端口: %d\n", *config.ProxyPort)
			}
		}
	}
	_ = scanner.Err()

	fmt.Println()
	fmt.Println(strings.Repeat("─", 48))

	// 3. 获取本地 IP
	localIP := getLocalIP()
	fmt.Printf("  本地 IP 地址: %s\n", localIP)

	// 4. 生成/加载 TLS 证书
	cert, err := loadOrGenerateCert()
	if err != nil {
		fmt.Printf("  [ERROR] 证书生成失败: %v\n", err)
		return
	}

	// 构建后端目标 URL
	targetURL := fmt.Sprintf("http://localhost:%d", backendPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		fmt.Printf("  [ERROR] 目标 URL 解析失败: %v\n", err)
		return
	}

	// 5. 构建路由（健康检查 + 代理转发 + WebSocket 代理）
	mux := http.NewServeMux()

	// /health 健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheckHandler(w, r, targetURL)
	})

	// 代理转发（含 WebSocket 升级检测）
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleProxyRequest(w, r, target, targetURL)
	})

	// 包裹 CORS 中间件
	handler := corsMiddleware(mux)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", proxyPort),
		Handler:   handler,
		TLSConfig: tlsConfig,
	}

	// 6. 在 goroutine 中启动服务器
	go func() {
		fmt.Printf("  HTTPS 代理服务器已启动\n")
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			fmt.Printf("  [ERROR] 代理服务器异常: %v\n", err)
		}
	}()

	// 7. 显示访问链接
	fmt.Println(strings.Repeat("─", 48))
	fmt.Println()
	fmt.Println("  HTTPS 代理服务已就绪，可通过以下链接访问：")
	fmt.Println()
	fmt.Printf("  本地访问:   https://localhost:%d\n", proxyPort)
	fmt.Printf("  局域网访问: https://%s:%d\n", localIP, proxyPort)
	fmt.Println()
	fmt.Printf("  后端转发目标: %s\n", targetURL)
	fmt.Println()
	fmt.Println("  输入 q 或按 Ctrl+C 退出代理服务")
	fmt.Println(strings.Repeat("─", 48))

	// 8. 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 另起 goroutine 监听键盘输入 'q'
	quitCh := make(chan struct{})
	go func() {
		inputScanner := bufio.NewScanner(os.Stdin)
		for inputScanner.Scan() {
			if strings.TrimSpace(inputScanner.Text()) == "q" {
				close(quitCh)
				return
			}
		}
		// 忽略 stdin 扫描错误（正常情况下由 EOF 触发）
		_ = inputScanner.Err()
	}()

	select {
	case <-sigCh:
		fmt.Println("\n  接收到中断信号，正在关闭代理服务...")
	case <-quitCh:
		fmt.Println("\n  正在关闭代理服务...")
	}

	// 关闭服务器
	if err := server.Close(); err != nil {
		fmt.Printf("  [WARN] 关闭代理服务器时出错: %v\n", err)
	}
	fmt.Println("  代理服务已关闭")
}

// handleProxyRequest 处理代理请求，检测 WebSocket 升级并分流
// - 普通 HTTP 请求：使用 ReverseProxy 转发
// - WebSocket 升级请求：使用 TCP 隧道转发
func handleProxyRequest(w http.ResponseWriter, r *http.Request, target *url.URL, targetURL string) {
	// 检测 WebSocket 升级请求
	if isWebSocketUpgrade(r) {
		handleWebSocketProxy(w, r, targetURL)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(req *http.Request) {
		originalPath := req.URL.Path
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.Header.Set("X-Forwarded-Proto", "https")

		_, port, splitErr := net.SplitHostPort(r.Host)
		if splitErr != nil || port == "" {
			port = "443"
		}
		req.Header.Set("X-Forwarded-Port", port)

		fmt.Printf("  [PROXY] %s %s -> %s%s\n", req.Method, originalPath, targetURL, req.URL.Path)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		fmt.Printf("  [ERROR] 代理转发失败: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"无法连接到后端服务器","message":"%s"}`, err.Error())
	}

	proxy.ServeHTTP(w, r)
}

// ==== TLS 证书管理 ====

// loadOrGenerateCert 尝试从磁盘加载证书，不存在或过期则重新生成
func loadOrGenerateCert() (tls.Certificate, error) {
	// 优先尝试从磁盘加载
	cert, err := loadCertFromDisk()
	if err == nil {
		fmt.Printf("  从磁盘加载证书成功: %s\n", *config.CertFile)
		return cert, nil
	}
	fmt.Printf("  磁盘证书不可用 (%v)，将重新生成\n", err)

	// 生成新证书
	return generateAndSaveCert()
}

// loadCertFromDisk 从磁盘加载证书并验证有效性
func loadCertFromDisk() (tls.Certificate, error) {
	certPEM, err := os.ReadFile(*config.CertFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取证书文件失败: %w", err)
	}
	keyPEM, err := os.ReadFile(*config.KeyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取私钥文件失败: %w", err)
	}

	// 解析证书检查有效期
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return tls.Certificate{}, fmt.Errorf("证书 PEM 解析失败")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("证书解析失败: %w", err)
	}

	// 检查是否过期（预留 7 天提前刷新）
	if time.Now().After(parsedCert.NotAfter.Add(-7 * 24 * time.Hour)) {
		return tls.Certificate{}, fmt.Errorf("证书即将过期或已过期")
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// generateAndSaveCert 生成新的自签名证书并保存到磁盘
func generateAndSaveCert() (tls.Certificate, error) {
	// 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}

	// 收集 IP 地址用于证书 SAN
	var ipAddresses []net.IP
	ipAddresses = append(ipAddresses, net.ParseIP("127.0.0.1"))
	ipAddresses = append(ipAddresses, net.ParseIP("::1"))

	localIP := getLocalIP()
	if parsedIP := net.ParseIP(localIP); parsedIP != nil {
		ipAddresses = append(ipAddresses, parsedIP)
	}

	// 生成证书序列号
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成序列号失败: %w", err)
	}

	// 创建证书模板
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Lunar Astral Agents"},
			CommonName:   "localhost",
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: ipAddresses,
		DNSNames:    []string{"localhost"},
	}

	// 创建自签名证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("创建证书失败: %w", err)
	}

	// 编码为 PEM 格式
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	// 保存到磁盘
	if err := saveCertToDisk(certPEM, keyPEM); err != nil {
		fmt.Printf("  [WARN] 证书持久化失败: %v（证书仅在内存中可用）\n", err)
	} else {
		fmt.Printf("  证书已持久化: %s\n", *config.CertFile)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// saveCertToDisk 将证书和私钥写入磁盘
func saveCertToDisk(certPEM, keyPEM []byte) error {
	certDirPath := filepath.Dir(*config.CertFile)
	if err := os.MkdirAll(certDirPath, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	if err := os.WriteFile(*config.CertFile, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书文件失败: %w", err)
	}

	if err := os.WriteFile(*config.KeyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("写入私钥文件失败: %w", err)
	}

	return nil
}

// ==== CORS 自动处理中间件 ====

// corsMiddleware 为所有响应添加 CORS 头，并处理 OPTIONS 预检请求
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Origin, Upgrade, Connection, Sec-WebSocket-Key, Sec-WebSocket-Version, Sec-WebSocket-Protocol, Sec-WebSocket-Extensions")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, X-Request-Id")

		// 处理 OPTIONS 预检请求
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ==== /health 健康检查端点 ====

// healthCheckHandler 返回代理服务健康状态（JSON 格式）
func healthCheckHandler(w http.ResponseWriter, _ *http.Request, targetURL string) {
	backendReachable := checkBackendHealth(targetURL)
	status := "ok"
	httpStatus := http.StatusOK
	if !backendReachable {
		status = "degraded"
		httpStatus = http.StatusOK // 代理本身正常，只是后端不可达
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	fmt.Fprintf(w, `{"status":"%s","service":"environment_repair_proxy","backend":"%s","backend_reachable":%t,"timestamp":"%s"}`,
		status, targetURL, backendReachable, time.Now().Format(time.RFC3339))
}

// checkBackendHealth 检查后端 HTTP 服务是否可达
func checkBackendHealth(targetURL string) bool {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(targetURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// ==== WebSocket 代理 (WSS→WS) ====

// isWebSocketUpgrade 检测请求是否为 WebSocket 升级请求
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Connection")) == "upgrade" &&
		strings.ToLower(r.Header.Get("Upgrade")) == "websocket"
}

// handleWebSocketProxy 处理 WebSocket 代理：劫持客户端连接，建立到后端的 TCP 隧道，双向转发数据
func handleWebSocketProxy(w http.ResponseWriter, r *http.Request, targetURL string) {
	// 解析后端地址
	backendHost := strings.TrimPrefix(targetURL, "http://")
	backendAddr := backendHost
	if !strings.Contains(backendHost, ":") {
		backendAddr = backendHost + ":80"
	}

	// 建立到后端的 TCP 连接
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		fmt.Printf("  [ERROR] WebSocket 后端连接失败 (%s): %v\n", backendAddr, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"WebSocket后端连接失败","message":"%s"}`, err.Error())
		return
	}
	defer backendConn.Close()

	// 劫持客户端连接
	hj, ok := w.(http.Hijacker)
	if !ok {
		fmt.Printf("  [ERROR] 服务器不支持连接劫持\n")
		http.Error(w, "WebSocket proxy not supported", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		fmt.Printf("  [ERROR] 劫持客户端连接失败: %v\n", err)
		return
	}
	defer clientConn.Close()

	// 将原始 HTTP 升级请求转发到后端
	if err := r.Write(backendConn); err != nil {
		fmt.Printf("  [ERROR] 转发 WebSocket 升级请求失败: %v\n", err)
		return
	}

	// 刷新客户端缓冲区
	bufrw.Flush()

	fmt.Printf("  [PROXY] WSS→WS 隧道已建立: %s -> %s\n", r.URL.Path, backendAddr)

	// 双向数据转发
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(backendConn, clientConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(clientConn, backendConn)
		done <- struct{}{}
	}()

	// 等待任一方向关闭
	<-done

	// 关闭连接
	backendConn.Close()
	clientConn.Close()

	fmt.Printf("  [PROXY] WSS→WS 隧道已关闭: %s\n", r.URL.Path)
}

// ==== 网络工具 ====

// getLocalIP 获取本机首选局域网 IP 地址
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	// 回退：尝试获取 IPv4 回环地址
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}

	return "localhost"
}
