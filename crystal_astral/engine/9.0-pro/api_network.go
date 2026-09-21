package ltp9

// ==== engine.network 裸 TCP/UDP/DNS（allow-network） ====

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/dop251/goja"
)

// networkDefaultTimeout 套接字读/连接的默认超时（秒）。
const networkDefaultTimeout = 5

// netSock 一个 TCP 或 UDP 底层套接字。
type netSock struct {
	tcp  net.Conn
	udp  *net.UDPConn
	addr *net.UDPAddr // udpListen 时记录的远端（sendTo 目标可被覆盖）
}

// networkTimeout 把「秒」参数归一为 time.Duration。
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

// netDialUDP 建立已「连接」的 UDP 套接字。
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

// netListenUDP 在指定端口监听 UDP。
func netListenUDP(host string, port int) (*netSock, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host), Port: port})
	if err != nil {
		return nil, err
	}
	return &netSock{udp: conn}, nil
}

// ==== 字节工具 ====

// valueToBytes 把 goja 值转换为字节数组。
func valueToBytes(v goja.Value) []byte {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	switch x := v.Export().(type) {
	case []byte:
		return x
	case goja.ArrayBuffer:
		return x.Bytes()
	case []interface{}:
		b := make([]byte, len(x))
		for i, n := range x {
			var val int
			switch t := n.(type) {
			case int64:
				val = int(t)
			case float64:
				val = int(t)
			}
			if val < 0 {
				val = 0
			}
			if val > 255 {
				val = 255
			}
			b[i] = byte(val)
		}
		return b
	case string:
		return []byte(x)
	default:
		return []byte(v.String())
	}
}

// byteResult 把收到的字节区段包装为 { success, data }（data 为整数数组）。
func byteResult(data []byte) any {
	ints := make([]int, len(data))
	for i, b := range data {
		ints[i] = int(b)
	}
	return map[string]any{"success": true, "data": ints}
}

// ==== 套接字读写方法 ====

// netSockSend 向连接写入字节数据。
func (s *netSock) netSockSend(data goja.Value) any {
	if s == nil {
		return rwResult{Success: false, Error: "套接字为空"}
	}
	payload := valueToBytes(data)
	if s.tcp != nil {
		_, err := s.tcp.Write(payload)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		return rwResult{Success: true}
	}
	if s.udp != nil {
		var err error
		switch {
		case s.addr != nil:
			_, err = s.udp.WriteToUDP(payload, s.addr)
		case s.udp.RemoteAddr() != nil:
			_, err = s.udp.Write(payload)
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

// netSockReceive 阻塞读取一段原始字节（带超时，秒）。
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
		return byteResult(buf[:n])
	}
	if s.udp != nil {
		_ = s.udp.SetReadDeadline(to)
		if s.udp.RemoteAddr() != nil {
			n, err := s.udp.Read(buf)
			if err != nil {
				return rwResult{Success: false, Error: err.Error()}
			}
			return byteResult(buf[:n])
		}
		n, from, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			return rwResult{Success: false, Error: err.Error()}
		}
		out := byteResult(buf[:n]).(map[string]any)
		out["host"] = from.IP.String()
		out["port"] = from.Port
		return out
	}
	return rwResult{Success: false, Error: "套接字未初始化"}
}

// netSockReceiveString 阻塞读取一段文本（与 receive 同源，仅按 UTF-8 转字符串）。
func (s *netSock) netSockReceiveString(timeoutSec float64) any {
	res, ok := s.netSockReceive(timeoutSec).(map[string]any)
	if !ok || res["success"] != true {
		return res
	}
	bytes, _ := res["data"].([]int)
	str := make([]byte, len(bytes))
	for i, b := range bytes {
		str[i] = byte(b)
	}
	return map[string]any{"success": true, "data": string(str)}
}

// netSockSendTo 向指定目标发送 UDP 字节。
func (s *netSock) netSockSendTo(host string, port int, data goja.Value) any {
	if s == nil || s.udp == nil {
		return rwResult{Success: false, Error: "非 UDP 套接字"}
	}
	if s.udp.RemoteAddr() != nil {
		return rwResult{Success: false, Error: "已连接 UDP 请用 send(data)"}
	}
	target := &net.UDPAddr{IP: net.ParseIP(host), Port: port}
	if _, err := s.udp.WriteToUDP(valueToBytes(data), target); err != nil {
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

// netSockObject 把底层套接字包装为暴露给 JS 的对象。
func netSockObject(vm *goja.Runtime, s *netSock) *goja.Object {
	obj := vm.NewObject()
	obj.Set("send", func(data goja.Value) any { return s.netSockSend(data) })
	obj.Set("receive", func(timeoutSec float64) any { return s.netSockReceive(timeoutSec) })
	obj.Set("receiveString", func(timeoutSec float64) any { return s.netSockReceiveString(timeoutSec) })
	obj.Set("sendTo", func(host string, port int, data goja.Value) any { return s.netSockSendTo(host, port, data) })
	obj.Set("close", func() any { return s.netSockClose() })
	return obj
}

// bindNet engine.network.*（allow-network）。
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