package proxy

// proxy.go 代理生命周期与请求处理

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// activeProxy 当前运行中的代理句柄（HTTPS 代理为进程级单例，IsRunning 的判据）
var activeProxy atomic.Pointer[Handle]

// IsRunning HTTPS 代理服务是否正在运行。
// 远程前端（手机等）经该代理接入 WebSocket，TTS 等音频消费方以
// 「代理运行中」作为远程前端在线的旁证。
func IsRunning() bool {
	return activeProxy.Load() != nil
}

// reservedPortOwner 本程序自身服务占用的端口 → 用途说明（由宿主注入）。
//
// 代理端口若落在这些端口上，会与自身服务抢端口：Windows 允许多次绑定，代理会把该端口
// 悄悄接管，导致原有服务（尤其是模型服务）对外表现异常。启动前必须拦下并说清原因。
var (
	reservedPortsMu sync.RWMutex
	reservedPorts   = map[int]string{}
)

// SetReservedPorts 注入本程序自身服务占用的端口（宿主启动时调用）
func SetReservedPorts(ports map[int]string) {
	reservedPortsMu.Lock()
	defer reservedPortsMu.Unlock()
	reservedPorts = make(map[int]string, len(ports))
	for p, desc := range ports {
		if p > 0 {
			reservedPorts[p] = desc
		}
	}
}

// ReservedPortOwner 返回该端口被本程序哪个服务占用（未占用则返回空串）
func ReservedPortOwner(port int) string {
	reservedPortsMu.RLock()
	defer reservedPortsMu.RUnlock()
	return reservedPorts[port]
}

// portConflictWarning 端口与自身服务冲突时的警告文案（空串表示不冲突）
func portConflictWarning(port int) string {
	if owner := ReservedPortOwner(port); owner != "" {
		return fmt.Sprintf("代理端口 %d 与本程序「%s」端口相同：会与它抢端口（Windows 允许多次绑定，"+
			"代理会接管该端口，导致该服务对外异常）", port, owner)
	}
	return ""
}

// suggestProxyPort 为冲突端口给出一个候选（避开自身服务占用的端口）
func suggestProxyPort(occupied int) int {
	reservedPortsMu.RLock()
	defer reservedPortsMu.RUnlock()
	for p := occupied + 1; p < occupied+100 && p <= 65535; p++ {
		if _, taken := reservedPorts[p]; !taken {
			return p
		}
	}
	return occupied + 1
}

// Start 启动 HTTPS 反向代理服务（TLS + 路由 + WebSocket 隧道），立即返回运行句柄。
// backendPort 为后端 HTTP 端口；proxyPort 为对外 HTTPS 端口；localIP 为证书 SAN 与二维码使用的本机 IP。
func Start(backendPort, proxyPort int, localIP string) (*Handle, Info, error) {
	// 端口自检：拒绝与本程序自身服务撞车，拒绝前后端口相同
	if proxyPort == backendPort {
		return nil, Info{}, fmt.Errorf("代理端口与后端端口不能相同（都是 %d）", proxyPort)
	}
	if owner := ReservedPortOwner(proxyPort); owner != "" {
		return nil, Info{}, fmt.Errorf("代理端口 %d 是本程序「%s」的端口，请改用其它端口（如 %d）",
			proxyPort, owner, suggestProxyPort(proxyPort))
	}
	if owner := ReservedPortOwner(backendPort); owner == "" {
		// 后端端口正常应指向 Web 服务；指到别的服务上属于误配，但不阻断（用户可能有意为之）
		logf("  [WARN] 后端端口 %d 不是本程序 Web 服务端口，代理将把请求转发到该端口", backendPort)
	}

	// 生成/加载 TLS 证书（确保 SAN 覆盖所选 IP）
	cert, err := loadOrGenerateCert(localIP)
	if err != nil {
		return nil, Info{}, fmt.Errorf("证书生成失败: %w", err)
	}

	// 构建后端目标 URL
	targetURL := fmt.Sprintf("http://localhost:%d", backendPort)
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, Info{}, fmt.Errorf("目标 URL 解析失败: %w", err)
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
		// 错误日志接入统一日志通道，避免默认 log 带时间戳直接刷控制台
		ErrorLog: log.New(&serverErrorWriter{}, "", 0),
	}

	certPath, _ := getCertPaths()
	info := Info{
		BackendPort: backendPort,
		ProxyPort:   proxyPort,
		IP:          localIP,
		QRText:      fmt.Sprintf("https://%s:%d", localIP, proxyPort),
	}
	logf("  证书来源: %s", certPath)

	handle := &Handle{srv: server, done: make(chan struct{})}

	// 先独占绑定端口：抢占别人的端口（Windows 允许多次绑定）必须变成明确失败，
	// 否则代理会悄悄接管该端口上原有的服务（实测：占用模型服务端口后，
	// 程序自己的模型请求打到 TLS 代理上，报 TLS 握手错误与连接重置）。
	ln, err := listenExclusive(proxyPort)
	if err != nil {
		return nil, Info{}, err
	}
	// 端口与本程序其它服务冲突时给出针对性提示：这类误配必然造成整机功能异常
	if warn := portConflictWarning(proxyPort); warn != "" {
		logf("  [WARN] %s", warn)
	}

	go func() {
		defer close(handle.done)
		// 监听器已独占；证书来自 TLSConfig，故文件参数留空
		if err := server.ServeTLS(ln, "", ""); err != nil && err != http.ErrServerClosed {
			logf("  [ERROR] 代理服务异常退出: %v", err)
		}
		activeProxy.CompareAndSwap(handle, nil)
	}()

	// 短暂等待以暴露端口占用等启动期错误
	time.Sleep(150 * time.Millisecond)
	select {
	case <-handle.done:
		return nil, Info{}, fmt.Errorf("代理服务启动失败（端口 %d 可能被占用）", proxyPort)
	default:
	}
	logf("  [PROXY] 服务已启动: %s → %s", info.QRText, targetURL)
	activeProxy.Store(handle)
	return handle, info, nil
}

// Stop 优雅停止代理服务
func (h *Handle) Stop() error {
	if h == nil || h.srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.srv.Shutdown(ctx); err != nil {
		// 收尾超时（多为仍有未结束的普通 HTTP 连接，WSS 隧道不参与等待）：
		// 监听器已在 Shutdown 中关闭，端口必然释放，此处仅记录并继续等待协程退出
		logf("  [WARN] 代理服务停止超时（%v），监听已关闭，端口已释放", err)
	}
	<-h.done
	activeProxy.CompareAndSwap(h, nil)
	logf("  [PROXY] 服务已停止")
	return nil
}

// ==== 服务器错误日志分流 ====

// serverErrorWriter 将 http.Server 的错误日志接入统一日志通道，
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
			certPath, _ := getCertPaths()
			logf("  [WARN] TLS 握手被拒绝：客户端尚未信任自签名证书（需安装并信任 %s 后访问）", filepath.Base(certPath))
		}
		return len(p), nil
	}

	logf("  [ERROR] %s", msg)
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

		logf("  [PROXY] %s %s -> %s%s", req.Method, originalPath, targetURL, req.URL.Path)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		logf("  [ERROR] 代理转发失败: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"无法连接到后端服务器","message":"%s"}`, err.Error())
	}

	proxy.ServeHTTP(w, r)
}

// ==== TLS 证书管理 ====

// loadOrGenerateCert 尝试从磁盘加载证书，不存在、过期或 SAN 未覆盖所需 IP 时重新生成
