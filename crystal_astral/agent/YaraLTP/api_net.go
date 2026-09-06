package YaraLTP

// ==== 网络类 API：http / network / platform ====

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// bindHTTP 注入 yara.http（get/post/download）。download 需插件上下文，
// 以便图片保存到插件自身 data 目录（与 readData/loadValid 的路径语义一致）。
func bindHTTP(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)

	objSetFn(o, "get", func(call goja.FunctionCall) goja.Value {
		url, headers, timeout := httpArgs(call, []int{1, 2})
		return vm.ToValue(doHTTPGet(url, headers, timeout))
	})
	objSetFn(o, "post", func(call goja.FunctionCall) goja.Value {
		url := argString(call, 0)
		body := argExport(call, 1)
		headers, timeout := httpHeadersTimeout(call, []int{2, 3})
		_ = body
		return vm.ToValue(doHTTPPost(url, argExport(call, 1), headers, timeout))
	})
	objSetFn(o, "download", func(call goja.FunctionCall) goja.Value {
		url := argString(call, 0)
		savePath := argString(call, 1)
		timeout := httpTimeoutArg(call, 2)
		return vm.ToValue(doHTTPDownload(url, savePath, timeout, p.DataDir))
	})

	parent.Set("http", o)
}

// httpArgs 解析 get 的 (url, [headers], [timeout])。
func httpArgs(call goja.FunctionCall, numIdx []int) (url string, headers map[string]string, timeout int) {
	url = argString(call, 0)
	headers = map[string]string{}
	timeout = httpDefaultTimeout
	for _, i := range numIdx {
		if i >= len(call.Arguments) {
			continue
		}
		v := call.Argument(i)
		if v.ExportType() != nil {
			ex := v.Export()
			if n, ok := toNum(ex); ok {
				timeout = int(n)
				continue
			}
			if m, ok := ex.(map[string]any); ok {
				headers = strMap(m)
			}
		}
	}
	return
}

// httpHeadersTimeout 解析 post 的 headers/timeout（在 body 之后）。
func httpHeadersTimeout(call goja.FunctionCall, idxs []int) (map[string]string, int) {
	headers := map[string]string{}
	timeout := httpDefaultTimeout
	for _, i := range idxs {
		if i >= len(call.Arguments) {
			continue
		}
		v := call.Argument(i)
		ex := v.Export()
		if n, ok := toNum(ex); ok {
			timeout = int(n)
			continue
		}
		if m, ok := ex.(map[string]any); ok {
			headers = strMap(m)
		}
	}
	return headers, timeout
}

// httpTimeoutArg 取最后一个数字参数作为超时。
func httpTimeoutArg(call goja.FunctionCall, idx int) int {
	t := httpDefaultTimeout
	if n := argInt(call, idx); n > 0 {
		return int(n)
	}
	return t
}

// toNum 判断导出值是否为数值。
func toNum(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	}
	return 0, false
}

// strMap 把 map[string]any 转为 map[string]string。
func strMap(m map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		switch t := v.(type) {
		case string:
			out[k] = t
		default:
			out[k] = fmt.Sprintf("%v", v)
		}
	}
	return out
}

// blockSSRF 判断目标主机是否命中需防护的本地/内网地址，命中返回 true。
func blockSSRF(host string) bool {
	host = strings.TrimSpace(host)
	lower := strings.ToLower(host)
	if strings.HasPrefix(lower, "localhost") || strings.HasPrefix(lower, "127.") ||
		lower == "::1" || strings.HasPrefix(lower, "0.0.0.0") {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return true
	}
	return false
}

func doHTTPGet(url string, headers map[string]string, timeoutSec int) any {
	if blockSSRF(hostOf(url)) {
		return map[string]any{"error": "禁止访问本地/内网地址"}
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return doHTTP(req, headers, timeoutSec)
}

func doHTTPPost(url string, body any, headers map[string]string, timeoutSec int) any {
	if blockSSRF(hostOf(url)) {
		return map[string]any{"error": "禁止访问本地/内网地址"}
	}
	var rd io.Reader
	switch b := body.(type) {
	case nil:
		rd = nil
	case []byte:
		rd = bytes.NewReader(b)
	case string:
		rd = strings.NewReader(b)
	default:
		if data, err := jsonEncode(b); err == nil {
			rd = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequest(http.MethodPost, url, rd)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return doHTTP(req, headers, timeoutSec)
}

func doHTTP(req *http.Request, headers map[string]string, timeoutSec int) any {
	if timeoutSec <= 0 {
		timeoutSec = httpDefaultTimeout
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	h := map[string]string{}
	for k := range resp.Header {
		h[k] = resp.Header.Get(k)
	}
	return map[string]any{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"body":       string(body),
		"headers":    h,
	}
}

func doHTTPDownload(url, savePath string, timeoutSec int, dataDir string) any {
	if blockSSRF(hostOf(url)) {
		return map[string]any{"error": "禁止访问本地/内网地址"}
	}
	if timeoutSec <= 0 {
		timeoutSec = httpDefaultTimeout
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	// 默认文件名取 URL 末段；空 savePath 时退化。
	if savePath == "" {
		savePath = filepath.Base(strings.TrimRight(url, "/"))
	}
	// 保存到插件自身 data 目录（与 readData/loadValid 的路径语义一致），
	// 避免图片落盘到全局 downloads 目录导致插件读不到新文件。
	full, err := safeResolve(dataDir, "data", savePath)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return map[string]any{"error": err.Error()}
	}
	if err := os.WriteFile(full, data, 0644); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return map[string]any{"success": true, "path": full, "size": len(data), "fileSize": len(data)}
}

// hostOf 从 URL 提取 host（含端口）。
func hostOf(url string) string {
	if i := strings.Index(url, "://"); i >= 0 {
		rest := url[i+3:]
		if j := strings.IndexAny(rest, "/?#"); j >= 0 {
			rest = rest[:j]
		}
		if k := strings.LastIndex(rest, "@"); k >= 0 {
			rest = rest[k+1:]
		}
		return rest
	}
	return url
}

// bindNetwork 注入 yara.network（TCP/UDP/DNS）。
func bindNetwork(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)

	objSetFn(o, "resolveDNS", func(call goja.FunctionCall) goja.Value {
		host := argString(call, 0)
		names, err := net.LookupHost(strings.TrimSpace(host))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(names)
	})
	objSetFn(o, "resolveSRV", func(call goja.FunctionCall) goja.Value {
		service := argString(call, 0)
		proto := argString(call, 1)
		name := argString(call, 2)
		_, addrs, err := net.LookupSRV(service, proto, name)
		if err != nil || len(addrs) == 0 {
			return vm.ToValue(map[string]any{"error": "SRV 记录未找到"})
		}
		sort.Slice(addrs, func(i, j int) bool { return addrs[i].Priority < addrs[j].Priority })
		return vm.ToValue(map[string]any{"target": addrs[0].Target, "port": int(addrs[0].Port)})
	})
	objSetFn(o, "tcpConnect", func(call goja.FunctionCall) goja.Value {
		host := argString(call, 0)
		port := int(argInt(call, 1))
		timeout := argInt(call, 2)
		if timeout <= 0 {
			timeout = 10
		}
		if blockSSRF(host) {
			return vm.ToValue(map[string]any{"error": "禁止连接本地/内网地址"})
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Duration(timeout)*time.Second)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(buildTCPSocket(vm, conn))
	})
	objSetFn(o, "udpConnect", func(call goja.FunctionCall) goja.Value {
		host := argString(call, 0)
		port := int(argInt(call, 1))
		timeout := argInt(call, 2)
		if timeout <= 0 {
			timeout = 10
		}
		if blockSSRF(host) {
			return vm.ToValue(map[string]any{"error": "禁止连接本地/内网地址"})
		}
		conn, err := net.DialTimeout("udp", net.JoinHostPort(host, strconv.Itoa(port)), time.Duration(timeout)*time.Second)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(buildUDPSocket(vm, conn, true))
	})
	objSetFn(o, "udpListen", func(call goja.FunctionCall) goja.Value {
		host := argString(call, 0)
		if host == "" {
			host = "0.0.0.0"
		}
		port := int(argInt(call, 1))
		conn, err := net.ListenPacket("udp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		uc, ok := conn.(*net.UDPConn)
		if !ok {
			return vm.ToValue(map[string]any{"error": "仅支持 UDP"})
		}
		return vm.ToValue(buildUDPListen(vm, uc))
	})

	parent.Set("network", o)
}

// jsSocket 网络 socket 包装（共享状态）。
type jsSocket struct {
	mu   sync.Mutex
	conn net.Conn
}

// buildTCPSocket 构建 TCP socket 对象。
func buildTCPSocket(vm *goja.Runtime, conn net.Conn) *goja.Object {
	s := &jsSocket{conn: conn}
	o := newObj(vm)
	objSetFn(o, "send", func(call goja.FunctionCall) goja.Value {
		data, ok := toBytes(argExport(call, 0))
		if !ok {
			return vm.ToValue(map[string]any{"error": "无法解析二进制数据"})
		}
		s.mu.Lock()
		_, err := s.conn.Write(data)
		s.mu.Unlock()
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "sendString", func(call goja.FunctionCall) goja.Value {
		s.mu.Lock()
		_, err := s.conn.Write([]byte(argString(call, 0)))
		s.mu.Unlock()
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "receive", func(call goja.FunctionCall) goja.Value {
		b, err := s.readUntil(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(intSlice(b))
	})
	objSetFn(o, "receiveString", func(call goja.FunctionCall) goja.Value {
		b, err := s.readUntil(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(string(b))
	})
	objSetFn(o, "close", func(call goja.FunctionCall) goja.Value {
		s.mu.Lock()
		err := s.conn.Close()
		s.mu.Unlock()
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	return o
}

// buildUDPSocket 构建已连接 UDP socket（send/receive/close）。
func buildUDPSocket(vm *goja.Runtime, conn net.Conn, connected bool) *goja.Object {
	o := buildTCPSocket(vm, conn)
	if uc, ok := conn.(*net.UDPConn); ok {
		objSetFn(o, "sendTo", func(call goja.FunctionCall) goja.Value {
			data, ok := toBytes(argExport(call, 0))
			if !ok {
				return vm.ToValue(map[string]any{"error": "无法解析二进制数据"})
			}
			host := argString(call, 1)
			port := int(argInt(call, 2))
			_, err := uc.WriteTo(data, &net.UDPAddr{IP: net.ParseIP(host), Port: port})
			if err != nil {
				return vm.ToValue(map[string]any{"error": err.Error()})
			}
			return vm.ToValue(map[string]any{"success": true})
		})
		objSetFn(o, "sendToString", func(call goja.FunctionCall) goja.Value {
			host := argString(call, 1)
			port := int(argInt(call, 2))
			_, err := uc.WriteTo([]byte(argString(call, 0)), &net.UDPAddr{IP: net.ParseIP(host), Port: port})
			if err != nil {
				return vm.ToValue(map[string]any{"error": err.Error()})
			}
			return vm.ToValue(map[string]any{"success": true})
		})
	}
	_ = connected
	return o
}

// buildUDPListen 构建监听型 UDP socket（含 receiveFrom）。
func buildUDPListen(vm *goja.Runtime, conn *net.UDPConn) *goja.Object {
	s := &udpListener{conn: conn}
	o := newObj(vm)
	objSetFn(o, "send", func(call goja.FunctionCall) goja.Value {
		data, ok := toBytes(argExport(call, 0))
		if !ok {
			return vm.ToValue(map[string]any{"error": "无法解析二进制数据"})
		}
		_, err := s.lastTargetWrite(data)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "sendString", func(call goja.FunctionCall) goja.Value {
		_, err := s.lastTargetWrite([]byte(argString(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "sendTo", func(call goja.FunctionCall) goja.Value {
		data, ok := toBytes(argExport(call, 0))
		if !ok {
			return vm.ToValue(map[string]any{"error": "无法解析二进制数据"})
		}
		host := argString(call, 1)
		port := int(argInt(call, 2))
		_, err := conn.WriteTo(data, &net.UDPAddr{IP: net.ParseIP(host), Port: port})
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "sendToString", func(call goja.FunctionCall) goja.Value {
		host := argString(call, 1)
		port := int(argInt(call, 2))
		_, err := conn.WriteTo([]byte(argString(call, 0)), &net.UDPAddr{IP: net.ParseIP(host), Port: port})
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "receive", func(call goja.FunctionCall) goja.Value {
		buf, addr, err := s.read(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		s.mu.Lock()
		s.lastTarget = addr
		s.mu.Unlock()
		return vm.ToValue(intSlice(buf))
	})
	objSetFn(o, "receiveString", func(call goja.FunctionCall) goja.Value {
		buf, addr, err := s.read(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		s.mu.Lock()
		s.lastTarget = addr
		s.mu.Unlock()
		return vm.ToValue(string(buf))
	})
	objSetFn(o, "receiveFrom", func(call goja.FunctionCall) goja.Value {
		buf, addr, err := s.read(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"data": intSlice(buf), "host": ipStr(addr), "port": addr.(*net.UDPAddr).Port})
	})
	objSetFn(o, "receiveFromString", func(call goja.FunctionCall) goja.Value {
		buf, addr, err := s.read(int(argInt(call, 0)))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"data": string(buf), "host": ipStr(addr), "port": addr.(*net.UDPAddr).Port})
	})
	objSetFn(o, "close", func(call goja.FunctionCall) goja.Value {
		if err := conn.Close(); err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"success": true})
	})
	objSetFn(o, "localAddr", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(conn.LocalAddr().String())
	})
	return o
}

// udpListener 监听型 UDP socket 状态。
type udpListener struct {
	mu         sync.Mutex
	conn       *net.UDPConn
	lastTarget net.Addr
}

func (u *udpListener) lastTargetWrite(data []byte) (int, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.lastTarget == nil {
		return 0, fmt.Errorf("尚无来源地址")
	}
	return u.conn.WriteTo(data, u.lastTarget)
}

func (u *udpListener) read(timeoutSec int) ([]byte, net.Addr, error) {
	_ = u.conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))
	buf := make([]byte, 65535)
	n, addr, err := u.conn.ReadFrom(buf)
	if err != nil {
		return nil, nil, err
	}
	return buf[:n], addr, nil
}

// readUntil 从 TCP 连接读缓冲直到遇到换行或超时。
func (s *jsSocket) readUntil(timeoutSec int) ([]byte, error) {
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	_ = s.conn.SetReadDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 1024)
	for {
		n, err := s.conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			if bytes.ContainsRune(tmp[:n], '\n') {
				break
			}
			if len(buf) >= 1<<20 {
				break
			}
		}
		if err != nil {
			if len(buf) > 0 {
				break
			}
			return nil, err
		}
	}
	return buf, nil
}

// toBytes 把 JS 传入的字符串/整数数组/[]byte 转为字节。
func toBytes(v any) ([]byte, bool) {
	switch t := v.(type) {
	case string:
		return []byte(t), true
	case []byte:
		return t, true
	case []any:
		out := make([]byte, 0, len(t))
		for _, e := range t {
			if n, ok := toNum(e); ok {
				out = append(out, byte(int(n)))
			} else {
				return nil, false
			}
		}
		return out, true
	case []int:
		out := make([]byte, 0, len(t))
		for _, n := range t {
			out = append(out, byte(n))
		}
		return out, true
	}
	return nil, false
}

// intSlice 把 []byte 转为 []int（align d.ts 的 number[]）。
func intSlice(b []byte) []any {
	out := make([]any, 0, len(b))
	for _, c := range b {
		out = append(out, int(c))
	}
	return out
}

// ipStr 把 net.Addr 转可读 host。
func ipStr(addr net.Addr) string {
	if a, ok := addr.(*net.UDPAddr); ok {
		return a.IP.String()
	}
	return addr.String()
}

// bindPlatform 注入 yara.platform。
func bindPlatform(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "sendCommand", func(call goja.FunctionCall) goja.Value {
		// 平台命令需真实客户端承接，引擎层仅占位（返回未接入）。
		return vm.ToValue(map[string]any{"success": false, "error": "平台命令未接入，属于真实客户端能力"})
	})
	objSetFn(o, "getName", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue("crystal_astral")
	})
	objSetFn(o, "getGroupId", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue("")
	})
	objSetFn(o, "lookupUser", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(nil)
	})
	parent.Set("platform", o)
}

// jsonEncode 安全 JSON 编码。
func jsonEncode(v any) ([]byte, error) {
	return json.Marshal(v)
}
