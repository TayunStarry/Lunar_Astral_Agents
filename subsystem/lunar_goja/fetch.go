package LunarGoja

import (
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// ==== fetch（标准 Fetch API） ====

// fetch 标准 Fetch API 网络请求函数（Promise 异步，遵循浏览器 fetch 调用形态）
// 用法:
//   const resp = await fetch(url, {
//     method: 'GET',              // 默认 GET
//     headers: { ... },           // 值可为字符串或字符串数组
//     body: {...} | 'string',     // 对象自动 JSON 序列化
//     timeout: 30,                // 秒，默认取 DefaultFetchTimeout
//     crossDomain: false,         // 跨域请求头
//     redirect: 'follow',         // 或 'manual' 不跟随重定向
//     rejectUnauthorized: false   // 默认跳过 TLS 校验（兼容本地自签名服务）
//   });
// Response 属性: status / statusText / ok / url(重定向后) / redirected / headers
// Response 方法: await resp.text() / await resp.json() / await resp.arrayBuffer()
// 网络错误时 reject(Error)；HTTP 非 2xx 不 reject（与标准一致，由调用方检查 resp.ok）
func (env *standardEnv) fetch(call goja.FunctionCall) goja.Value {
	runtime := env.runtime
	if len(call.Arguments) < 1 {
		panic(runtime.NewTypeError("fetch: url 参数缺失"))
	}
	urlStr := call.Argument(0).String()

	// 解析可选项（非对象时忽略）
	opts := map[string]any{}
	if len(call.Arguments) > 1 {
		if m, ok := call.Argument(1).Export().(map[string]any); ok {
			opts = m
		}
	}

	promise, resolve, reject := runtime.NewPromise()
	go func() {
		status, statusText, headerMap, body, finalURL, redirected, err := doFetch(urlStr, opts)
		// 回到事件循环线程 resolve/reject（goja Promise 非 goroutine-safe）
		RunOnLoop(func(vm *goja.Runtime) {
			if err != nil {
				LoggerGeneral.Error("LunarGoja", "fetch 请求失败(%s): %v", urlStr, err)
				reject(vm.NewGoError(err))
				return
			}
			resolve(buildFetchResponse(vm, status, statusText, headerMap, body, finalURL, redirected))
		})
	}()
	// *goja.Promise 需经 ToValue 转换后才能作为 goja.Value 返回给 JS
	return runtime.ToValue(promise)
}

// doFetch 在独立 goroutine 中执行 HTTP 请求并返回原始响应数据
func doFetch(urlStr string, opts map[string]any) (int, string, http.Header, []byte, string, bool, error) {
	method := "GET"
	if v, ok := opts["method"].(string); ok && v != "" {
		method = strings.ToUpper(v)
	}

	// 请求体：字符串直发；对象自动 JSON 序列化
	var body io.Reader
	if bodyVal, ok := opts["body"]; ok && bodyVal != nil {
		switch v := bodyVal.(type) {
		case string:
			body = bytes.NewBufferString(v)
		case []byte:
			body = bytes.NewBuffer(v)
		default:
			bodyJSON, err := json.Marshal(v)
			if err != nil {
				return 0, "", nil, nil, "", false, fmt.Errorf("请求体序列化失败: %v", err)
			}
			body = bytes.NewBuffer(bodyJSON)
		}
	}

	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}

	// 请求头：值可为字符串或字符串数组
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

	// 跨域请求头
	if crossDomain, ok := opts["crossDomain"].(bool); ok && crossDomain {
		req.Header.Set("Origin", "*")
		req.Header.Set("Access-Control-Allow-Origin", "*")
		req.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	// 超时：默认使用 DefaultFetchTimeout
	timeout := DefaultFetchTimeout
	if t, ok := opts["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t * float64(time.Second))
	}

	// TLS 校验：默认跳过（兼容本地自签名服务），可经 rejectUnauthorized 开启
	rejectUnauthorized := false
	if v, ok := opts["rejectUnauthorized"].(bool); ok {
		rejectUnauthorized = v
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: !rejectUnauthorized}}

	// 重定向策略：默认跟随；redirect: 'manual' 时不跟随
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	if redirect, ok := opts["redirect"].(string); ok && redirect == "manual" {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", nil, nil, "", false, err
	}

	finalURL := resp.Request.URL.String()
	redirected := finalURL != urlStr
	return resp.StatusCode, resp.Status, resp.Header, responseBody, finalURL, redirected, nil
}

// buildFetchResponse 构建标准 Response 对象（含 Headers 与 text/json/arrayBuffer 方法）
func buildFetchResponse(vm *goja.Runtime, status int, statusText string, headerMap http.Header, body []byte, finalURL string, redirected bool) *goja.Object {
	respObj := vm.NewObject()
	respObj.Set("status", status)
	respObj.Set("statusText", statusText)
	respObj.Set("ok", status >= 200 && status < 300)
	respObj.Set("url", finalURL)
	respObj.Set("redirected", redirected)
	respObj.Set("headers", buildHeadersObject(vm, headerMap))

	// text() → Promise<string>
	respObj.Set("text", func(goja.FunctionCall) goja.Value {
		p, res, _ := vm.NewPromise()
		res(vm.ToValue(string(body)))
		return vm.ToValue(p)
	})

	// json() → Promise<any>，解析失败 reject
	respObj.Set("json", func(goja.FunctionCall) goja.Value {
		p, res, rej := vm.NewPromise()
		var data any
		if err := json.Unmarshal(body, &data); err != nil {
			rej(vm.NewGoError(fmt.Errorf("JSON 解析失败: %v", err)))
			return vm.ToValue(p)
		}
		res(vm.ToValue(data))
		return vm.ToValue(p)
	})

	// arrayBuffer() → Promise<ArrayBuffer>
	respObj.Set("arrayBuffer", func(goja.FunctionCall) goja.Value {
		p, res, _ := vm.NewPromise()
		res(vm.NewArrayBuffer(body))
		return vm.ToValue(p)
	})

	return respObj
}

// buildHeadersObject 构建标准 Headers 对象（get/has/entries/raw，键大小写不敏感）
func buildHeadersObject(vm *goja.Runtime, headerMap http.Header) *goja.Object {
	// 统一转为小写键的多值映射
	lower := make(map[string][]string, len(headerMap))
	for key, values := range headerMap {
		lower[strings.ToLower(key)] = values
	}

	headersObj := vm.NewObject()
	headersObj.Set("get", func(call goja.FunctionCall) goja.Value {
		values, ok := lower[strings.ToLower(call.Argument(0).String())]
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(strings.Join(values, ", "))
	})
	headersObj.Set("has", func(call goja.FunctionCall) goja.Value {
		_, ok := lower[strings.ToLower(call.Argument(0).String())]
		return vm.ToValue(ok)
	})
	headersObj.Set("entries", func(goja.FunctionCall) goja.Value {
		pairs := make([]any, 0, len(lower))
		for key, values := range lower {
			pairs = append(pairs, []any{key, strings.Join(values, ", ")})
		}
		return vm.ToValue(pairs)
	})
	headersObj.Set("raw", func(goja.FunctionCall) goja.Value {
		raw := make(map[string]any, len(lower))
		for key, values := range lower {
			raw[key] = values
		}
		return vm.ToValue(raw)
	})
	return headersObj
}
