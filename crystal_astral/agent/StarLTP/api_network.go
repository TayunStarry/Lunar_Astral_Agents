package StarLTP

// ==== engine.network 裸 TCP/UDP/DNS（allow-network） ====
// 依 LTP9 同步阻塞模型实现：connect/listen 返回「网络套接字对象」，其 send/receive
// 等为阻塞调用（读取带超时），在插件事件循环线程内执行。WebSocket 能力见全局 WebSocket
// 与 engine.ws，此处覆盖 WebSocket 替代不了的非 HTTP 场景。

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/dop251/goja"
)

// networkDefaultTimeout 套接字读/连接的默认超时（秒，未显式传参时）。
const networkDefaultTimeout = 5

// netSock 一个 TCP 或 UDP 底层套接字（持有底层 net.Conn / net.UDPConn）。
type netSock struct {
	tcp  net.Conn
	udp  *net.UDPConn
	addr *net.UDPAddr // udpListen 时记录的远端（sendTo 目标可被覆盖）
}

// networkTimeout 把「秒」参数归一为 time.Duration；<=0 用默认值（统一以秒为单位）。
func networkTimeout(sec float64) time.Duration {
	if sec <= 0 {
		return networkDefaultTimeout * time.Second
	}
	return time.Duration(sec * float64(time.Second))
}

// netResolveDNS 解析主机名的全部 IPv4/IPv6 地址。
func netResolveDNS(host string, timeoutSec float64) any {
	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout(timeoutSec))
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}
	}
	return map[string]any{"success": true, "addresses": addrs}
}

// netResolveSRV 按服务/协议/主机解析 SRV 记录。
func netResolveSRV(service, proto, host string, timeoutSec float64) any {
	ctx, cancel := context.WithTimeout(context.Background(), networkTimeout(timeoutSec))
	defer cancel()
	_, records, err := net.DefaultResolver.LookupSRV(ctx, service, proto, host)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}
	}
	out := make([]map[string]any, 0, len(records))
	for _, r := range records {
		out = append(out, map[string]any{"target": r.Target, "port": r.Port})
	}
	return map[string]any{"success": true, "targets": out}
}

// netDialTCP 建立 TCP 连接，返回套接字对象。
func netDialTCP(host string, port int, timeoutSec float64) (*netSock, error) {
	d := net.Dialer{Timeout: networkTimeout(timeoutSec)}
	conn, err := d.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return nil, err
	}
	return &netSock{tcp: conn}, nil
}

// netDialUDP 建立已「连接」的 UDP 套接字（单向发往固定目标）。
func netDialUDP(host string, port int, timeoutSec float64) (*netSock, error) {
	d := net.Dialer{Timeout: networkTimeout(timeoutSec)}
	conn, err := d.Dial("udp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return nil, err
	}
	u, ok := conn.(*net.UDPConn)
	if !ok {
		conn.Close()
		return nil, fmt.Errorf("非 UDP 连接")
	}
	return &netSock{udp: u}, nil
}

// netListenUDP 在指定端口监听 UDP，返回套接字（可 sendTo/receiveFrom）。
func netListenUDP(host string, port int) (*netSock, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host), Port: port})
	if err != nil {
		return nil, err
	}
	return &netSock{udp: conn}, nil
}

// ==== 套接字读写方法 ====

// netSockSend 以文本形式向连接写入数据。
// TCP 直接 Write；UDP 按连接态分派：已「连接」（udpConnect，RemoteAddr 非空）用 Write 发往固定对端，
// 未连接且已记录远端（udpListen + sendTo）用 WriteToUDP；两者皆无则报错提示用 sendTo。
func (s *netSock) netSockSend(data string) any {
	if s == nil {
		return rwResult{Success: false, Error: "套接字为空"}
	}
	if s.tcp != nil {
		_, err := s.tcp.Write([]byte(data))
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return rwResult{Success: true}
	}
	if s.udp != nil {
		var err error
		switch {
		case s.addr != nil:
			_, err = s.udp.WriteToUDP([]byte(data), s.addr)
		case s.udp.RemoteAddr() != nil:
			_, err = s.udp.Write([]byte(data))
		default:
			return rwResult{Success: false, Error: "UDP 未指定目标，请用 sendTo"}
		}
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return rwResult{Success: true}
	}
	return rwResult{Success: false, Error: "套接字未初始化"}
}

// netSockReceive 阻塞读取一段文本（带超时，秒）。
func (s *netSock) netSockReceive(timeoutSec float64) any {
	if s == nil {
		return rwResult{Success: false, Error: "套接字为空"}
	}
	to := time.Now().Add(networkTimeout(timeoutSec))
	buf := make([]byte, 65536)
	if s.tcp != nil {
		_ = s.tcp.SetReadDeadline(to)
		n, err := s.tcp.Read(buf)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return map[string]any{"success": true, "data": string(buf[:n])}
	}
	if s.udp != nil {
		_ = s.udp.SetReadDeadline(to)
		// 已连接（udpConnect）：Read 读固定对端（无 host/port）；未连接（udpListen）：ReadFromUDP 带对端信息
		if s.udp.RemoteAddr() != nil {
			n, err := s.udp.Read(buf)
			if err != nil {
				return rwResult{Success: false, Error: err.Error()}
			}
			return map[string]any{"success": true, "data": string(buf[:n])}
		}
		n, from, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return map[string]any{"success": true, "data": string(buf[:n]), "host": from.IP.String(), "port": from.Port}
	}
	return rwResult{Success: false, Error: "套接字未初始化"}
}

// netSockSendTo 向指定目标发送 UDP 数据（udpListen 场景；已连接 UDP 请用 send）。
func (s *netSock) netSockSendTo(host string, port int, data string) any {
	if s == nil || s.udp == nil {
		return rwResult{Success: false, Error: "非 UDP 套接字"}
	}
	if s.udp.RemoteAddr() != nil {
		return rwResult{Success: false, Error: "已连接 UDP 请用 send(data)"}
	}
	target := &net.UDPAddr{IP: net.ParseIP(host), Port: port}
	if _, err := s.udp.WriteToUDP([]byte(data), target); err != nil {
		return rwResult{Success: false, Error: err.Error()}
	}
	return rwResult{Success: true}
}

// netSockClose 关闭套接字。
func (s *netSock) netSockClose() any {
	if s == nil {
		return rwResult{Success: true}
	}
	if s.tcp != nil {
		_ = s.tcp.Close()
	}
	if s.udp != nil {
		_ = s.udp.Close()
	}
	return rwResult{Success: true}
}

// netSockObject 把底层套接字包装为暴露给 JS 的对象（send/receive/sendTo/receiveFrom/close）。
func netSockObject(vm *goja.Runtime, s *netSock) *goja.Object {
	obj := vm.NewObject()
	obj.Set("send", func(data string) any { return s.netSockSend(data) })
	obj.Set("receive", func(timeoutSec float64) any { return s.netSockReceive(timeoutSec) })
	obj.Set("sendTo", func(host string, port int, data string) any { return s.netSockSendTo(host, port, data) })
	obj.Set("close", func() any { return s.netSockClose() })
	return obj
}

// bindNet engine.network.*（allow-network）：DNS 解析 + TCP/UDP 连接与监听。
func bindNet(vm *goja.Runtime) *goja.Object {
	n := vm.NewObject()
	n.Set("resolveDNS", func(host string, timeoutSec float64) any { return netResolveDNS(host, timeoutSec) })
	n.Set("resolveSRV", func(service, proto, host string, timeoutSec float64) any {
		return netResolveSRV(service, proto, host, timeoutSec)
	})
	n.Set("tcpConnect", func(host string, port int, timeoutSec float64) any {
		s, err := netDialTCP(host, port, timeoutSec)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return netSockObject(vm, s)
	})
	n.Set("udpConnect", func(host string, port int, timeoutSec float64) any {
		s, err := netDialUDP(host, port, timeoutSec)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return netSockObject(vm, s)
	})
	n.Set("udpListen", func(host string, port int) any {
		s, err := netListenUDP(host, port)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return netSockObject(vm, s)
	})
	return n
}
