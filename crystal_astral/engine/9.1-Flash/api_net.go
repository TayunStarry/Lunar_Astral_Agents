package StarLTP

// ==== 沙箱网络：同步 fetch / WebSocket 客户端与同步 http ====
// fetch 为同步形态（参照 lunar_astral engine.syncFetch 方案）：阻塞直到返回统一
// Result 信封 { success, status, ok, url, headers, body, error? }，body 为 JSON 时自动
// 解析为对象；WebSocket 客户端以 onopen/onmessage 等事件回调形态提供（见 api_ws.go）。
// 同步形态 http.get/post/download 由 binder 按 allow-network 注入。全部网络能力由
// allow-network 门控。

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// fetchTimeout 一次 fetch 的默认超时。
// 图像生成（如 Seedream 2K）耗时可达 40~120s，放宽默认超时避免误判客户端超时（status 0）。
const fetchTimeout = 150 * time.Second

// bindNetwork 把 fetch 与 WebSocket 客户端全局注册进插件沙箱（allow-network 门控，由 bindSandbox 调用）。
func bindNetwork(vm *goja.Runtime, p *plugin) {
	// 同步 fetch：阻塞直到返回统一 Result 信封（Result 风格与 http.get/post 同构）
	vm.Set("fetch", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.NewTypeError("fetch: url 参数缺失"))
		}
		urlStr := call.Argument(0).String()
		opts := map[string]any{}
		if len(call.Arguments) > 1 {
			if m, ok := call.Argument(1).Export().(map[string]any); ok {
				opts = m
			}
		}
		status, _, headerMap, body, finalURL, _, err := doLTP9Fetch(urlStr, opts)
		if err != nil {
			LoggerGeneral.Error(ServiceName, "插件 %s fetch 失败(%s): %v", p.ID, urlStr, err)
			return vm.ToValue(map[string]any{"success": false, "status": 0, "error": err.Error()})
		}
		// 响应头压平为小写键单值 map
		hdrs := map[string]any{}
		for key, values := range headerMap {
			hdrs[strings.ToLower(key)] = strings.Join(values, ", ")
		}
		// body 为 JSON 时自动解析为对象（syncFetch 方案），失败回退原始文本
		var parsed any
		bodyVal := any(string(body))
		if json.Unmarshal(body, &parsed) == nil {
			bodyVal = parsed
		}
		return vm.ToValue(map[string]any{
			"success": true,
			"status":  status,
			"ok":      status >= 200 && status < 300,
			"url":     finalURL,
			"headers": hdrs,
			"body":    bodyVal,
		})
	})
	// WebSocket 客户端全局（新）。
	bindWebSocket(vm, p)
}

// httpGetSync 同步 GET：阻塞直到返回，回 { success, status, body } 或 { success:false, error }。供不用异步的插件使用。
func httpGetSync(url string, headers map[string]any) map[string]any {
	opts := map[string]any{"method": "GET", "timeout": float64(fetchTimeout / time.Second)}
	if len(headers) > 0 {
		opts["headers"] = headers
	}
	status, _, _, body, _, _, err := doLTP9Fetch(url, opts)
	if err != nil {
		return map[string]any{"success": false, "status": 0, "body": "", "error": err.Error()}
	}
	return map[string]any{"success": true, "status": status, "body": string(body)}
}

// httpPostSync 同步 POST：阻塞直到返回，返回 { success, status, body } 或 { success:false, error }。
func httpPostSync(url, body string, headers map[string]any) map[string]any {
	opts := map[string]any{"method": "POST", "body": body, "timeout": float64(fetchTimeout / time.Second)}
	if len(headers) > 0 {
		opts["headers"] = headers
	}
	status, _, _, b, _, _, err := doLTP9Fetch(url, opts)
	if err != nil {
		return map[string]any{"success": false, "status": 0, "body": "", "error": err.Error()}
	}
	return map[string]any{"success": true, "status": status, "body": string(b)}
}

// httpDownloadSync 同步下载文件到插件数据目录（http.download），savePath 为空时按 URL 末段命名。
func httpDownloadSync(p *plugin, rawURL, savePath string) map[string]any {
	if p == nil {
		return map[string]any{"success": false, "error": "插件无效"}
	}
	// savePath 为空时按 URL 末段命名，非法则退化为 download.bin
	if savePath == "" {
		if u, uerr := url.Parse(rawURL); uerr == nil {
			name := path.Base(u.Path)
			if name == "." || name == "/" || name == "" {
				name = "download.bin"
			}
			savePath = name
		} else {
			savePath = "download.bin"
		}
	}
	real, serr := scopedPath(p, savePath)
	if serr != nil {
		return map[string]any{"success": false, "error": serr.Error()}
	}
	status, _, _, body, _, _, err := doLTP9Fetch(rawURL, map[string]any{"method": "GET", "timeout": float64(fetchTimeout / time.Second)})
	if err != nil {
		return map[string]any{"success": false, "error": "下载失败: " + err.Error()}
	}
	if status < 200 || status >= 300 {
		return map[string]any{"success": false, "error": fmt.Sprintf("下载状态: %d", status)}
	}
	if werr := os.WriteFile(real, body, 0644); werr != nil {
		return map[string]any{"success": false, "error": werr.Error()}
	}
	return map[string]any{"success": true, "path": savePath, "size": len(body)}
}

// doLTP9Fetch 在独立 goroutine 里执行 HTTP 请求。
func doLTP9Fetch(urlStr string, opts map[string]any) (int, string, http.Header, []byte, string, bool, error) {
	method := "GET"
	if v, ok := opts["method"].(string); ok && v != "" {
		method = strings.ToUpper(v)
	}
	var body io.Reader
	if bodyVal, ok := opts["body"]; ok && bodyVal != nil {
		switch v := bodyVal.(type) {
		case string:
			body = bytes.NewBufferString(v)
		case []byte:
			body = bytes.NewBuffer(v)
		default:
			js, err := json.Marshal(v)
			if err != nil {
				return 0, "", nil, nil, "", false, fmt.Errorf("请求体序列化失败: %v", err)
			}
			body = bytes.NewBuffer(js)
		}
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	if headers, ok := opts["headers"].(map[string]any); ok {
		for key, value := range headers {
			switch v := value.(type) {
			case string:
				req.Header.Set(key, v)
			case []any:
				for _, item := range v {
					if s, ok := item.(string); ok {
						req.Header.Add(key, s)
					}
				}
			}
		}
	}
	timeout := fetchTimeout
	if t, ok := opts["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t * float64(time.Second))
	}
	// TLS 默认校验服务端证书（防 MITM 篡改插件消费的数据）；server.insecure_tls 显式置 true 才跳过
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: *GeneralConfig.InsecureTLS}}
	client := &http.Client{Timeout: timeout, Transport: transport}
	if redirect, ok := opts["redirect"].(string); ok && redirect == "manual" {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	defer resp.Body.Close()
	responseBody, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return 0, "", nil, nil, "", false, rerr
	}
	finalURL := resp.Request.URL.String()
	return resp.StatusCode, resp.Status, resp.Header, responseBody, finalURL, finalURL != urlStr, nil
}
