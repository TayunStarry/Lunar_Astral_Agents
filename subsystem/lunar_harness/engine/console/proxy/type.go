// Package proxy 提供 WSS→WS 反向代理能力（迁移自 environment_repair/proxy.go，
// 逻辑保持一致：TLS 证书管理 + 普通 HTTP 反向代理 + WebSocket TCP 隧道 + Host/Origin 改写）。
package proxy

import "net/http"

// Info 代理服务运行态信息（TUI 展示用）
type Info struct {
	BackendPort int    // 后端 HTTP 端口
	ProxyPort   int    // 代理 HTTPS 端口
	IP          string // 绑定 IP
	QRText      string // 二维码内容（局域网接入地址）
}

// ipCandidate 可选的本机 IP 地址候选（含所属网卡名）
type ipCandidate struct {
	IP        string // IPv4 地址
	Interface string // 所属网卡名称
}

// Handle 运行中的代理服务句柄（Stop 用于优雅停止）
type Handle struct {
	srv  *http.Server
	done chan struct{} // Serve 退出信号
}
