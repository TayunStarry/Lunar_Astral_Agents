package main

import (
	"LunarSubsystem/GeneralConfig"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ==== HTTPS 代理服务器 ====

// startProxyServer 构建 HTTPS 代理服务器（TLS 证书 + 路由），由 TUI 代理屏调用
// 返回已配置但未启动的 server，调用方自行在 goroutine 中执行 ListenAndServeTLS
func startProxyServer(backendPort, proxyPort int, localIP string) (*http.Server, proxyInfo, error) {
	// 生成/加载 TLS 证书（确保 SAN 覆盖所选 IP）
	cert, err := loadOrGenerateCert(localIP)
	if err != nil {
		return nil, proxyInfo{}, fmt.Errorf("证书生成失败: %w", err)
	}

	// 构建后端目标 URL
	targetURL := fmt.Sprintf("http://localhost:%d", backendPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, proxyInfo{}, fmt.Errorf("目标 URL 解析失败: %w", err)
	}

	// 构建路由（健康检查 + 代理转发 + WebSocket 代理）
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		healthCheckHandler(w, r, targetURL)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleProxyRequest(w, r, target, targetURL)
	})

	server := &http.Server{
		Addr:      fmt.Sprintf(":%d", proxyPort),
		Handler:   corsMiddleware(mux),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		// 错误日志接入 TUI 统一通道，避免默认 log 带时间戳直接刷控制台
		ErrorLog: log.New(&serverErrorWriter{}, "", 0),
	}

	lanURL := ""
	if localIP != "localhost" {
		lanURL = fmt.Sprintf("https://%s:%d", localIP, proxyPort)
	}
	info := proxyInfo{
		LocalURL: fmt.Sprintf("https://localhost:%d", proxyPort),
		LANURL:   lanURL,
		Target:   targetURL,
		CertInfo: fmt.Sprintf("证书: %s", *GeneralConfig.CertFile),
	}
	return server, info, nil
}

// ==== 服务器错误日志分流 ====

// serverErrorWriter 将 http.Server 的错误日志接入 TUI 统一日志通道，
// 并对客户端未信任自签证书导致的握手拒绝做限频降噪（这是自签证书下的预期行为，
// 不应作为系统错误高频刷屏）
type serverErrorWriter struct {
	certErrMu      sync.Mutex
	certErrLastLog time.Time
}

func (w *serverErrorWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))

	// 客户端不信任自签证书 → 预期现象，限频为每 30 秒至多一条并附带处理指引
	if strings.Contains(msg, "unknown certificate") || strings.Contains(msg, "bad certificate") {
		w.certErrMu.Lock()
		shouldLog := time.Since(w.certErrLastLog) > 30*time.Second
		if shouldLog {
			w.certErrLastLog = time.Now()
		}
		w.certErrMu.Unlock()

		if shouldLog {
			uiLogRaw("  [WARN] TLS 握手被拒绝：客户端尚未信任自签名证书" +
				"（手机端需安装并信任 " + *GeneralConfig.CertFile + " 后访问）")
		}
		return len(p), nil
	}

	uiLogRaw("  [ERROR] " + msg)
	return len(p), nil
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

		uiLogf("  [PROXY] %s %s -> %s%s", req.Method, originalPath, targetURL, req.URL.Path)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		uiLogf("  [ERROR] 代理转发失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"无法连接到后端服务器","message":"%s"}`, err.Error())
	}

	proxy.ServeHTTP(w, r)
}

// ==== TLS 证书管理 ====

// loadOrGenerateCert 尝试从磁盘加载证书，不存在、过期或 SAN 未覆盖所需 IP 时重新生成
func loadOrGenerateCert(requiredIP string) (tls.Certificate, error) {
	// 优先尝试从磁盘加载
	cert, err := loadCertFromDisk(requiredIP)
	if err == nil {
		uiLogf("  从磁盘加载证书成功: %s", *GeneralConfig.CertFile)
		return cert, nil
	}
	uiLogf("  磁盘证书不可用 (%v)，将重新生成", err)

	// 生成新证书
	return generateAndSaveCert(requiredIP)
}

// loadCertFromDisk 从磁盘加载证书并验证有效性与 IP 覆盖
func loadCertFromDisk(requiredIP string) (tls.Certificate, error) {
	certPEM, err := os.ReadFile(*GeneralConfig.CertFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("读取证书文件失败: %w", err)
	}
	keyPEM, err := os.ReadFile(*GeneralConfig.KeyFile)
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

	// 检查证书 SAN 是否覆盖所需 IP（回环与 localhost 除外）
	if !certCoversIP(parsedCert, requiredIP) {
		return tls.Certificate{}, fmt.Errorf("证书 SAN 未覆盖 IP %s", requiredIP)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// certCoversIP 检查证书 SAN 是否包含指定 IP（localhost/回环地址仅检查 DNS/回环覆盖）
func certCoversIP(cert *x509.Certificate, ipStr string) bool {
	if ipStr == "localhost" {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() {
		return true
	}
	for _, certIP := range cert.IPAddresses {
		if certIP.Equal(ip) {
			return true
		}
	}
	return false
}

// generateAndSaveCert 生成新的自签名证书并保存到磁盘，SAN 覆盖本机全部 IPv4 地址与所需 IP
func generateAndSaveCert(requiredIP string) (tls.Certificate, error) {
	// 生成 RSA 私钥
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}

	// 收集 IP 地址用于证书 SAN：回环 + 所有所选 IP + 本机全部可用 IPv4
	ipSet := make(map[string]net.IP)
	ipSet["127.0.0.1"] = net.ParseIP("127.0.0.1")
	ipSet["::1"] = net.ParseIP("::1")
	if parsed := net.ParseIP(requiredIP); parsed != nil {
		ipSet[requiredIP] = parsed
	}
	for _, candidate := range listLocalIPs() {
		if parsed := net.ParseIP(candidate.IP); parsed != nil {
			ipSet[candidate.IP] = parsed
		}
	}

	var ipAddresses []net.IP
	for _, ip := range ipSet {
		ipAddresses = append(ipAddresses, ip)
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
		uiLogf("  [WARN] 证书持久化失败: %v（证书仅在内存中可用）", err)
	} else {
		uiLogf("  证书已持久化: %s", *GeneralConfig.CertFile)
	}

	return tls.X509KeyPair(certPEM, keyPEM)
}

// saveCertToDisk 将证书和私钥写入磁盘
func saveCertToDisk(certPEM, keyPEM []byte) error {
	certDirPath := filepath.Dir(*GeneralConfig.CertFile)
	if err := os.MkdirAll(certDirPath, 0755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}

	if err := os.WriteFile(*GeneralConfig.CertFile, certPEM, 0644); err != nil {
		return fmt.Errorf("写入证书文件失败: %w", err)
	}

	if err := os.WriteFile(*GeneralConfig.KeyFile, keyPEM, 0600); err != nil {
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
// Connection 头可能是 "Upgrade" 或 "keep-alive, Upgrade" 等多 token 形式，需按 token 列表解析
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, value := range r.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// handleWebSocketProxy 处理 WebSocket 代理：劫持客户端连接，建立到后端的 TCP 隧道，双向转发数据
func handleWebSocketProxy(w http.ResponseWriter, r *http.Request, targetURL string) {
	// 解析后端地址（targetURL 形如 http://host:port）
	backendHost := strings.TrimPrefix(strings.TrimPrefix(targetURL, "https://"), "http://")
	backendAddr := backendHost
	if !strings.Contains(backendHost, ":") {
		backendAddr = backendHost + ":80"
	}

	// 建立到后端的 TCP 连接
	backendConn, err := net.DialTimeout("tcp", backendAddr, 10*time.Second)
	if err != nil {
		uiLogf("  [ERROR] WebSocket 后端连接失败 (%s): %v", backendAddr, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"WebSocket后端连接失败","message":"%s"}`, err.Error())
		return
	}
	defer backendConn.Close()

	// 劫持客户端连接
	hj, ok := w.(http.Hijacker)
	if !ok {
		uiLogf("  [ERROR] 服务器不支持连接劫持")
		http.Error(w, "WebSocket proxy not supported", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		uiLogf("  [ERROR] 劫持客户端连接失败: %v", err)
		return
	}
	defer clientConn.Close()

	// 改写 Host 头为后端地址后再转发升级请求，避免后端对陌生 Host 校验失败
	r.Host = backendHost
	if err := r.Write(backendConn); err != nil {
		uiLogf("  [ERROR] 转发 WebSocket 升级请求失败: %v", err)
		return
	}

	uiLogf("  [PROXY] WSS→WS 隧道已建立: %s -> %s", r.URL.Path, backendAddr)

	// 双向数据转发
	// 客户端方向以 bufrw 为源：先排空 Hijack 时 bufio.Reader 中已缓冲的帧数据（通常为紧随升级请求的早期 WS 帧），再回落到底层连接
	done := make(chan struct{}, 2)

	go func() {
		io.Copy(backendConn, bufrw)
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

	uiLogf("  [PROXY] WSS→WS 隧道已关闭: %s", r.URL.Path)
}

// ==== 网络工具 ====

// listLocalIPs 枚举本机所有可用 IPv4 地址（附带网卡名，仅列出已启用的网卡）
func listLocalIPs() []ipCandidate {
	var candidates []ipCandidate
	seen := make(map[string]bool)

	ifaces, err := net.Interfaces()
	if err != nil {
		return candidates
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ip := ipNet.IP.To4(); ip != nil && !seen[ip.String()] {
					seen[ip.String()] = true
					candidates = append(candidates, ipCandidate{
						IP:        ip.String(),
						Interface: iface.Name,
					})
				}
			}
		}
	}
	return candidates
}

// ipScore 为 IP 地址打分用于自动选择：私网地址优先，链路本地 (169.254.*) 与回环最后
// 返回值越小越优先；-1 表示应跳过
func ipScore(ipStr string) int {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return -1
	}
	switch {
	case ip.IsLoopback():
		return 90
	case ip.IsLinkLocalUnicast(): // 169.254.0.0/16，通常是网卡未获得 DHCP 分配时的自动地址
		return 80
	case ip.IsPrivate(): // 192.168.0.0/16、10.0.0.0/8、172.16.0.0/12
		return 0
	default:
		return 50
	}
}

// ipCategory 描述 IP 地址类别，用于列表展示
func ipCategory(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "未知"
	}
	switch {
	case ip.IsLoopback():
		return "回环"
	case ip.IsLinkLocalUnicast():
		return "链路本地 (不可用于局域网访问)"
	case ip.IsPrivate():
		return "局域网"
	default:
		return "公网/其他"
	}
}
