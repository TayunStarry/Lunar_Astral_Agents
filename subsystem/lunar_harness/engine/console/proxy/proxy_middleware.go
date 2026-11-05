package proxy

// proxy_middleware.go 中间件与 WebSocket 转发

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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
	fmt.Fprintf(w, `{"status":"%s","service":"tui_system_proxy","backend":"%s","backend_reachable":%t,"timestamp":"%s"}`,
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
		logf("  [ERROR] WebSocket 后端连接失败 (%s): %v", backendAddr, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"WebSocket后端连接失败","message":"%s"}`, err.Error())
		return
	}
	defer backendConn.Close()

	// 劫持客户端连接
	hj, ok := w.(http.Hijacker)
	if !ok {
		logf("  [ERROR] 服务器不支持连接劫持")
		http.Error(w, "WebSocket proxy not supported", http.StatusInternalServerError)
		return
	}

	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		logf("  [ERROR] 劫持客户端连接失败: %v", err)
		return
	}
	defer clientConn.Close()

	// 改写 Host 头为后端地址后再转发升级请求，避免后端对陌生 Host 校验失败。
	// 同步改写 Origin 为改写后的后端主机：后端 Upgrader.CheckOrigin 要求
	// 「Origin 的主机 == 请求 Host」（同源校验）。代理已把 Host 换成后端地址，
	// 若不同步对齐 Origin，校验必然失败（403）→ 手机端 WSS 升级被拒。
	r.Host = backendHost
	if origin := r.Header.Get("Origin"); origin != "" {
		if u, err := url.Parse(origin); err == nil && u.Scheme != "" {
			r.Header.Set("Origin", u.Scheme+"://"+backendHost)
		} else {
			r.Header.Set("Origin", "http://"+backendHost)
		}
	}
	if err := r.Write(backendConn); err != nil {
		logf("  [ERROR] 转发 WebSocket 升级请求失败: %v", err)
		return
	}

	logf("  [PROXY] WSS→WS 隧道已建立: %s -> %s", r.URL.Path, backendAddr)

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

	logf("  [PROXY] WSS→WS 隧道已关闭: %s", r.URL.Path)
}

// ==== 网络工具 ====

// LocalIPs 枚举本机所有可用 IPv4 地址（按私网优先排序，仅列出已启用的网卡）
