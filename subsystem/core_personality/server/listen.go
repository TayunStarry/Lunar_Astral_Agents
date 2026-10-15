package server

// ==== 实例端口独占监听 ====
//
// 为什么需要它：Windows 的 bind 默认允许**同一端口被多个套接字绑定**（最后一个绑定者
// 收到新连接），而 Go 自 1.19 起在 Windows 上不再设置 SO_EXCLUSIVEADDRUSE
// （见 $GOROOT/src/net/sockopt_windows.go：只处理 IPV6_V6ONLY 与 SO_BROADCAST）。
// 结果是重复启动的两个实例都能"监听成功"：后启动者悄悄接管 HTTP 端口、两边各自拉起
// 一个模型服务并互抢端口，用户看到的正是「前端 404 → 前后端一起崩溃退出」。
//
// 因此实例端口必须显式独占绑定：拿不到它就意味着已有实例在跑，本进程在装配任何子系统
// **之前**就该给出明确提示并退出，而不是先起半个服务再莫名其妙地消失。

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// preboundMu 保护预绑定监听器（main 预检成功后注入，Start 直接复用）
var preboundMu sync.Mutex
var preboundLn net.Listener

// SetPreboundListener 注入已独占绑定的实例监听器。
// 由 main 在预检成功后调用，避免「预检关闭 → Start 重新绑定」之间出现竞态窗口。
func SetPreboundListener(ln net.Listener) {
	preboundMu.Lock()
	defer preboundMu.Unlock()
	preboundLn = ln
}

// takePreboundListener 取走预绑定监听器（只生效一次）
func takePreboundListener() net.Listener {
	preboundMu.Lock()
	defer preboundMu.Unlock()
	ln := preboundLn
	preboundLn = nil
	return ln
}

// ListenInstance 以独占方式绑定实例端口（Windows 上设置 SO_EXCLUSIVEADDRUSE）。
// 返回错误即表示端口已被占用——**极可能已有另一个实例在运行**。
func ListenInstance(port int, allowLAN bool) (net.Listener, error) {
	host := "127.0.0.1"
	if allowLAN {
		host = ""
	}
	lc := net.ListenConfig{Control: exclusiveAddrControl}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return nil, fmt.Errorf("端口 %d 已被占用（可能已有另一个「星月智能」实例在运行）: %w", port, err)
	}
	return ln, nil
}
