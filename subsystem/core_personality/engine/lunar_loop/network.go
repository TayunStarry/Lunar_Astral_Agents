package lunarloop

import (
	"LunarSubsystem/BrowserClient"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"LunarSubsystem/TuiSystem"
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

// getEnvironmentUrl 返回本机服务的可达基地址（供 JS 端自调用本机端点）。
// 返回值: [string, error] 系统URL和错误信息。
//
// 必须与服务端绑定策略保持一致：默认仅绑定 127.0.0.1（见 server.Start），
// 此时返回局域网 IP 会导致智能体自调用（/vram/unload、/vram/guard）连接被拒；
// 仅在 server.allow_lan 放开、服务监听全部网卡时，才返回局域网 IP。
func (class *Runtime) getEnvironmentUrl(call goja.FunctionCall) goja.Value {
	if !*GeneralConfig.AllowLAN {
		return class.runtime.ToValue([]any{GeneralConfig.LocalBaseURL(), nil})
	}
	ip, err := BrowserClient.GetLocalIP([]string{})
	if err != nil {
		LoggerGeneral.Error("LunarCore", "获取本地IP失败: %v", err)
		return class.runtime.ToValue([]any{GeneralConfig.LocalBaseURL(), nil})
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

	// 图片闸门：模型请求出站前逐条校验 image_url，超尺寸 / 非视觉读取产物一律拦截
	// （思维链只接受 /file/read-multimodal/ 端点或视觉副本，见 model_image_guard.go 与 docs/adr/0008）
	blockedImages, guardErr := guardChatRequestImages(url, execute)
	if guardErr != nil {
		return class.runtime.ToValue([]any{nil, guardErr})
	}
	for _, item := range blockedImages {
		LoggerGeneral.SubError(lunarCoreModule, lunarJavaScriptSub, "[图片闸门] 已拦截原图流转: %s", item)
	}

	// 流式桥检测：chat/completions + stream:true → 注入 include_usage 并转流式分支
	streaming := prepareStreamingBody(url, execute)

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

	// 角色标签：优先请求头 X-Lunar-Role，其次 config.roleTag（TUI 智能体实况屏显示用）
	role := "月华"
	if xr := req.Header.Get("X-Lunar-Role"); xr != "" {
		role = xr
	} else if rt, ok := config["roleTag"].(string); ok && rt != "" {
		role = rt
	}

	// 流式模型调用：Go 侧解析 SSE 增量并实时推送 TUI，同步返回组装后的完整响应
	if streaming {
		promptEstimate := 0
		if b, err := json.Marshal(execute["body"]); err == nil {
			promptEstimate = utf8.RuneCountInString(string(b))
		}
		return class.doStreamingChat(req, role, promptEstimate)
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

// prepareStreamingBody 检测流式模型请求并注入 stream_options.include_usage。
// 命中条件：URL 含 /chat/completions 且请求体 JSON 的 stream 字段为 true；
// 命中后请求体统一转为对象（重建 JSON 时带上 stream_options），返回 true 表示走流式分支。
func prepareStreamingBody(url string, execute map[string]any) bool {
	if !strings.Contains(url, "/chat/completions") {
		return false
	}
	bodyVal, ok := execute["body"]
	if !ok {
		return false
	}
	var obj map[string]any
	switch v := bodyVal.(type) {
	case map[string]any:
		obj = v
	case string:
		if json.Unmarshal([]byte(v), &obj) != nil {
			return false
		}
	default:
		return false
	}
	if stream, _ := obj["stream"].(bool); !stream {
		return false
	}
	if _, ok := obj["stream_options"]; !ok {
		obj["stream_options"] = map[string]any{"include_usage": true}
	}
	execute["body"] = obj
	return true
}

// doStreamingChat 流式模型调用桥：
//  1. 关闭整体超时（SSE 时长不可预估），仅保留握手超时防模型重载挂起；
//  2. 逐帧解析 SSE：思考/正文增量实时推送 TUI（StreamDelta），工具分片按 index 聚合；
//  3. usage 到账按真实 token 计量，缺失时按文本长度估算（estimated 标记）；
//  4. 组装完整 choices[0].message 非流式结构同步返回 JS，消费端零改动。
func (class *Runtime) doStreamingChat(req *http.Request, role string, promptEstimate int) goja.Value {
	TuiSystem.StreamBegin(role)
	start := time.Now()

	req.Header.Set("Accept", "text/event-stream")
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			ResponseHeaderTimeout: 5 * time.Minute,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		TuiSystem.StreamEnd(role)
		LoggerGeneral.SubError(lunarCoreModule, lunarJavaScriptSub, "流式模型调用失败: %v", err)
		return class.runtime.ToValue([]any{nil, fmt.Errorf("发送请求失败: %v", err)})
	}
	defer resp.Body.Close()

	// 非 200：按非流式约定返回原始内容（错误说明多在响应体中）
	if resp.StatusCode != http.StatusOK {
		TuiSystem.StreamEnd(role)
		raw, _ := io.ReadAll(resp.Body)
		var responseJSON map[string]any
		if json.Unmarshal(raw, &responseJSON) == nil {
			return class.runtime.ToValue([]any{map[string]any{
				"status": resp.StatusCode, "headers": resp.Header, "body": responseJSON,
			}, nil})
		}
		return class.runtime.ToValue([]any{map[string]any{
			"status": resp.StatusCode, "headers": resp.Header, "body": string(raw),
		}, nil})
	}

	content := &strings.Builder{}
	reasoning := &strings.Builder{}
	tools := newToolCallAccumulator()
	var usage *tokenUsage
	finishReason := ""

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk sseChunk
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, choice := range chunk.Choices {
			delta := choice.Delta
			if think := delta.ReasoningContent + delta.Reasoning; think != "" {
				reasoning.WriteString(think)
				TuiSystem.StreamDelta(role, TuiSystem.KindThink, think)
			}
			if delta.Content != "" {
				content.WriteString(delta.Content)
				TuiSystem.StreamDelta(role, TuiSystem.KindContent, delta.Content)
			}
			tools.ingest(delta.ToolCalls)
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				finishReason = *choice.FinishReason
			}
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}
	if err := scanner.Err(); err != nil {
		LoggerGeneral.SubWarn(lunarCoreModule, lunarJavaScriptSub, "流式响应中断: %v", err)
	}

	// 顺序要紧：工具调用必须先归到「本次模型调用」这一轮里，再结束该轮。
	// 反过来（先 StreamEnd 再 report）会让 StreamTool 在 currentTurn 为空时另起一轮，
	// 事件与现实就对不上了——实况屏会出现「只有一条工具调用、永远进行中」的幽灵轮次，
	// 而真正的思考/正文留在上一轮里，导出与界面都错位。
	tools.report(role)
	TuiSystem.StreamEnd(role)

	// 计量入账：优先真实 usage，缺失时按文本长度估算（prompt 取请求体字符数 / 3）
	if usage != nil {
		TuiSystem.RecordModelCall(usage.Prompt, usage.Completion, time.Since(start), false)
	} else {
		prompt := int64(promptEstimate) / 3
		completion := int64(utf8.RuneCountInString(content.String()) +
			utf8.RuneCountInString(reasoning.String()) +
			utf8.RuneCountInString(tools.argsText()))
		TuiSystem.RecordModelCall(prompt, completion, time.Since(start), true)
	}

	// 组装非流式等价响应（choices[0].message），JS 消费端零改动
	message := map[string]any{"role": "assistant", "content": content.String()}
	if reasoning.Len() > 0 {
		message["reasoning_content"] = reasoning.String()
	}
	if calls := tools.messageCalls(); len(calls) > 0 {
		message["tool_calls"] = calls
	}
	body := map[string]any{
		"choices": []any{map[string]any{
			"index":         0,
			"message":       message,
			"finish_reason": finishReason,
		}},
	}
	if usage != nil {
		body["usage"] = map[string]any{
			"prompt_tokens":     usage.Prompt,
			"completion_tokens": usage.Completion,
			"total_tokens":      usage.Prompt + usage.Completion,
		}
	}
	return class.runtime.ToValue([]any{map[string]any{
		"status":  resp.StatusCode,
		"headers": resp.Header,
		"body":    body,
	}, nil})
}

// newToolCallAccumulator 创建流式工具调用聚合器
func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{items: make(map[int]*toolAccum)}
}

// ingest 收纳一批 tool_calls 分片
func (t *toolCallAccumulator) ingest(calls []sseToolCall) {
	for _, c := range calls {
		item, ok := t.items[c.Index]
		if !ok {
			item = &toolAccum{id: c.ID}
			t.items[c.Index] = item
			t.order = append(t.order, c.Index)
		}
		if c.ID != "" {
			item.id = c.ID
		}
		if c.Function.Name != "" {
			item.name = c.Function.Name
		}
		if c.Function.Arguments != "" {
			item.args.WriteString(c.Function.Arguments)
		}
	}
}

// report 将聚合完成的工具调用序列上报 TUI（名称 + 参数摘要）
func (t *toolCallAccumulator) report(role string) {
	for _, idx := range t.order {
		item := t.items[idx]
		if item.name == "" {
			continue
		}
		TuiSystem.StreamTool(role, item.name, item.args.String())
	}
}

// argsText 返回全部工具参数拼接文本（估算 completion tokens 用）
func (t *toolCallAccumulator) argsText() string {
	var b strings.Builder
	for _, idx := range t.order {
		b.WriteString(t.items[idx].args.String())
	}
	return b.String()
}

// messageCalls 组装 OpenAI 非流式 message.tool_calls 结构
func (t *toolCallAccumulator) messageCalls() []any {
	if len(t.order) == 0 {
		return nil
	}
	calls := make([]any, 0, len(t.order))
	for _, idx := range t.order {
		item := t.items[idx]
		if item.name == "" {
			continue
		}
		calls = append(calls, map[string]any{
			"id":   item.id,
			"type": "function",
			"function": map[string]any{
				"name":      item.name,
				"arguments": item.args.String(),
			},
		})
	}
	return calls
}
