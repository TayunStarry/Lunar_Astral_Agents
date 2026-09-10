package StarLTP

// ==== 沙箱网络：全局 fetch / WebSocket 客户端与同步 engine.http ====
// fetch 为标准形态（url + init，返回 Promise<Response>），resolve/reject 排在插件自身的
// eventloop 上（p.loop），保证 Promise 在正确线程流转；WebSocket 客户端以 onopen/onmessage
// 等事件回调形态提供。同步形态 engine.http.get/post 由 binder 按 allow-network 注入。
// 全部网络能力由 allow-network 门控。

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

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// fetchTimeout 一次 fetch 的默认超时。
// 图像生成（如 Seedream 2K）耗时可达 40~120s，放宽默认超时避免误判客户端超时（status 0）。
const fetchTimeout = 150 * time.Second

// bindNetwork 把 fetch 与 WebSocket 客户端全局注册进插件沙箱（allow-network 门控，由 plugin.load 调用）。
func bindNetwork(vm *goja.Runtime, p *plugin) {
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
		promise, resolve, reject := vm.NewPromise()
		go func() {
			status, statusText, headerMap, body, finalURL, redirected, err := doLTP9Fetch(urlStr, opts)
			p.loop.RunOnLoop(func(rvm *goja.Runtime) {
				if err != nil {
					LoggerGeneral.Error(ServiceName, "插件 %s fetch 失败(%s): %v", p.ID, urlStr, err)
					reject(rvm.NewGoError(err))
					return
				}
				resolve(buildLTP9FetchResponse(rvm, status, statusText, headerMap, body, finalURL, redirected))
			})
		}()
		return vm.ToValue(promise)
	})
	// WebSocket 客户端全局（新）。
	bindWebSocket(vm, p)
}

// httpGetSync 同步 GET：阻塞直到返回，回 {status, body} 或 {error}。供不用异步的插件使用。
func httpGetSync(url string, headers map[string]any) map[string]any {
	opts := map[string]any{"method": "GET", "timeout": float64(fetchTimeout / time.Second)}
	if len(headers) > 0 {
		opts["headers"] = headers
	}
	status, _, _, body, _, _, err := doLTP9Fetch(url, opts)
	if err != nil {
		return map[string]any{"status": 0, "body": "", "error": err.Error()}
	}
	return map[string]any{"status": status, "body": string(body)}
}

// httpPostSync 同步 POST：阻塞直到返回，返回 {status, body} 或 {error}。
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

func httpPostSync(url, body string, headers map[string]any) map[string]any {
	opts := map[string]any{"method": "POST", "body": body, "timeout": float64(fetchTimeout / time.Second)}
	if len(headers) > 0 {
		opts["headers"] = headers
	}
	status, _, _, b, _, _, err := doLTP9Fetch(url, opts)
	if err != nil {
		return map[string]any{"status": 0, "body": "", "error": err.Error()}
	}
	return map[string]any{"status": status, "body": string(b)}
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
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
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

// buildLTP9FetchResponse 构建 Response 对象（status/ok/headers + text/json/arrayBuffer）。
func buildLTP9FetchResponse(vm *goja.Runtime, status int, statusText string, headerMap http.Header, body []byte, finalURL string, redirected bool) *goja.Object {
	resp := vm.NewObject()
	resp.Set("status", status)
	resp.Set("statusText", statusText)
	resp.Set("ok", status >= 200 && status < 300)
	resp.Set("url", finalURL)
	resp.Set("redirected", redirected)
	resp.Set("headers", buildLTP9Headers(vm, headerMap))

	resp.Set("text", func(goja.FunctionCall) goja.Value {
		p, res, _ := vm.NewPromise()
		res(vm.ToValue(string(body)))
		return vm.ToValue(p)
	})
	resp.Set("json", func(goja.FunctionCall) goja.Value {
		p, res, rej := vm.NewPromise()
		var data any
		if err := json.Unmarshal(body, &data); err != nil {
			rej(vm.NewGoError(fmt.Errorf("JSON 解析失败: %v", err)))
			return vm.ToValue(p)
		}
		res(vm.ToValue(data))
		return vm.ToValue(p)
	})
	resp.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
		p, res, _ := vm.NewPromise()
		res(vm.NewArrayBuffer(body))
		return vm.ToValue(p)
	})
	return resp
}

// buildLTP9Headers 构建 Headers 对象（get/has/entries/raw，小写键）。
func buildLTP9Headers(vm *goja.Runtime, headerMap http.Header) *goja.Object {
	lower := map[string][]string{}
	for key, values := range headerMap {
		lower[strings.ToLower(key)] = values
	}
	h := vm.NewObject()
	h.Set("get", func(call goja.FunctionCall) goja.Value {
		vals, ok := lower[strings.ToLower(call.Argument(0).String())]
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(strings.Join(vals, ", "))
	})
	h.Set("has", func(call goja.FunctionCall) goja.Value {
		_, ok := lower[strings.ToLower(call.Argument(0).String())]
		return vm.ToValue(ok)
	})
	h.Set("entries", func(goja.FunctionCall) goja.Value {
		pairs := make([]any, 0, len(lower))
		for key, values := range lower {
			pairs = append(pairs, []any{key, strings.Join(values, ", ")})
		}
		return vm.ToValue(pairs)
	})
	h.Set("raw", func(goja.FunctionCall) goja.Value {
		raw := map[string]any{}
		for key, values := range lower {
			raw[key] = values
		}
		return vm.ToValue(raw)
	})
	return h
}