package proxy

// ==== 代理监听：显式、独占 ====
//
// 为什么不能直接用 ListenAndServeTLS：
//
//  1. **Windows 允许同一端口被多次绑定**（后者接管新连接），而 Go 不再设置
//     SO_EXCLUSIVEADDRUSE（见 $GOROOT/src/net/sockopt_windows.go）。于是代理若选了
//     一个已被占用的端口（例如**模型服务端口**），它会「绑定成功」并把该端口悄悄接管：
//     程序自身的模型请求随即打到 TLS 代理上，报出
//     `client sent an HTTP request to an HTTPS server`、连接被重置、嵌入请求 unexpected EOF，
//     连锁导致记忆库查询失败。实测故障正是如此。
//  2. 先独占拿到监听器再对外服务，端口冲突会变成**一条明确的错误**，而不是静默劫持。

import (
	"context"
	"fmt"
	"net"
	"time"
)

// listenExclusive 以独占方式绑定端口（Windows 上设置 SO_EXCLUSIVEADDRUSE）。
// 返回错误即表示端口已被占用——包括被本程序自己的其它服务占用。
func listenExclusive(port int) (net.Listener, error) {
	lc := net.ListenConfig{Control: exclusiveAddrControl}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, fmt.Errorf("端口 %d 无法独占监听（可能已被占用，例如模型服务端口）: %w", port, err)
	}
	return ln, nil
}
