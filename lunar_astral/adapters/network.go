package adapters

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dop251/goja"
)

// address 适配TypeScript调用的网络地址查询功能，获取当前服务器网络地址列表
// 使用 ip-api.com 替代 ipapi.co，后者限流严格（频繁返回 429）
// 返回值: [Array<string>, error] 地址列表和错误信息
func (class *Runtime) address(call goja.FunctionCall) goja.Value {
	// 如果当前地址已缓存，直接返回
	if len(GeneralConfig.ServerAddress) > 0 {
		return class.runtime.ToValue([]any{GeneralConfig.ServerAddress, nil})
	}

	// 默认兜底地址
	defaultAddr := []string{"江苏省", "南京市"}

	// 调用 ip-api.com 查询位置信息（支持中文，免费版限流 45次/分钟，仅支持 HTTP）
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/?lang=zh-CN&fields=status,message,regionName,city")
	if err != nil {
		LoggerGeneral.Error("LunarCore", "获取位置失败: %v", err)
		return class.runtime.ToValue([]any{defaultAddr, err})
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		LoggerGeneral.Error("LunarCore", "获取位置失败: %s", resp.Status)
		return class.runtime.ToValue([]any{defaultAddr, fmt.Errorf("HTTP状态异常: %s", resp.Status)})
	}

	// 解析JSON响应
	var data IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		LoggerGeneral.Error("LunarCore", "解析位置信息失败: %v", err)
		return class.runtime.ToValue([]any{defaultAddr, err})
	}

	// 检查API返回状态
	if data.Status != "success" {
		LoggerGeneral.Error("LunarCore", "获取位置失败: %s", data.Message)
		return class.runtime.ToValue([]any{defaultAddr, fmt.Errorf("API返回失败: %s", data.Message)})
	}

	// 确保省份和城市信息存在
	if data.RegionName == "" || data.City == "" {
		LoggerGeneral.Error("LunarCore", "获取位置失败: 省份或城市信息缺失")
		return class.runtime.ToValue([]any{defaultAddr, fmt.Errorf("省份或城市信息缺失")})
	}

	// 缓存当前地址
	GeneralConfig.ServerAddress = []string{data.RegionName, data.City}
	return class.runtime.ToValue([]any{GeneralConfig.ServerAddress, nil})
}

// url 适配TypeScript调用的系统URL获取功能，返回系统访问地址
// 返回值: [string, error] 系统URL和错误信息
func (class *Runtime) url(call goja.FunctionCall) goja.Value {
	ip, err := BrowserClient.GetLocalIP([]string{})
	if err != nil {
		LoggerGeneral.Error("LunarCore", "获取本地IP失败: %v", err)
		return class.runtime.ToValue([]any{fmt.Sprintf("http://localhost:%d", *GeneralConfig.BasicPort), nil})
	}
	return class.runtime.ToValue([]any{fmt.Sprintf("http://%s:%d", ip, *GeneralConfig.BasicPort), nil})
}

// syncFetch 适配TypeScript调用的网络请求代理功能，处理HTTP请求并返回统一格式响应
// 返回值: [Object, error] 网络响应和错误信息
func (class *Runtime) syncFetch(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	config, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("config必须是对象")})
	}

	// 解析URL
	url, ok := config["url"].(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("无效的URL")})
	}

	// 解析执行配置
	execute, ok := config["execute"].(map[string]any)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("无效的execute")})
	}

	// 确定HTTP方法
	method := "GET"
	if methodVal, ok := execute["method"].(string); ok {
		method = methodVal
	}

	// 准备请求体
	var body io.Reader
	if bodyVal, ok := execute["body"]; ok {
		switch v := bodyVal.(type) {
		case string:
			body = bytes.NewBufferString(v)
		default:
			bodyJSON, err := json.Marshal(v)
			if err != nil {
				return class.runtime.ToValue([]any{nil, fmt.Errorf("请求体序列化失败: %v", err)})
			}
			body = bytes.NewBuffer(bodyJSON)
		}
	}

	// 创建HTTP请求
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("创建请求失败: %v", err)})
	}

	// 设置请求头
	if headers, ok := execute["headers"].(map[string]any); ok {
		for key, value := range headers {
			if valueStr, ok := value.(string); ok {
				req.Header.Set(key, valueStr)
			}
		}
	}

	// 处理跨域请求
	if crossDomain, ok := execute["crossDomain"].(bool); ok && crossDomain {
		req.Header.Set("Origin", "*")
		req.Header.Set("Access-Control-Allow-Origin", "*")
		req.Header.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		req.Header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	}

	// 发送请求（支持自签名证书，设置超时防止模型重载期间无限挂起）
	client := &http.Client{
		Timeout: time.Duration(*GeneralConfig.SyncFetchTimeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("发送请求失败: %v", err)})
	}
	defer resp.Body.Close()

	// 读取响应
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("读取响应失败: %v", err)})
	}

	// 解析响应为JSON
	var responseJSON map[string]any
	err = json.Unmarshal(responseBody, &responseJSON)
	if err != nil {
		// 非JSON响应返回原始内容
		response := map[string]any{
			"status":  resp.StatusCode,
			"headers": resp.Header,
			"body":    string(responseBody),
		}
		return class.runtime.ToValue([]any{response, nil})
	}

	// 返回JSON响应
	response := map[string]any{
		"status":  resp.StatusCode,
		"headers": resp.Header,
		"body":    responseJSON,
	}
	return class.runtime.ToValue([]any{response, nil})
}
