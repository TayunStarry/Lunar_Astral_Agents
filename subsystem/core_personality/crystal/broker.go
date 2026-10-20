package crystal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"CorePersonality/protocol/ltpx"
	"LunarSubsystem/LoggerGeneral"
)

// Broker LTPX 协调的进程内实现：
// 覆盖原 /ltpx/tools|call|poll|event 全部职责，直接调用同一批处理器，
// 不经 HTTP 通道、不设心跳、不判离线，调用方（月华）只依赖 protocol/ltpx.Broker 接口。
type Broker struct{}

// NewBroker 创建 LTPX 协调实现（由装配层注入到 protocol/ltpx）
func NewBroker() *Broker {
	return &Broker{}
}

// Status 返回当前聚合工具链（原 /ltpx/tools）。
// 合并为单进程后不存在在线/离线判定：工具链为空（扫描无 AtoA 能力包）时 Tool 为 nil。
func (b *Broker) Status() ltpx.Status {
	code, body := invokeEndpoint(LTPRemoteToolsHandler, http.MethodGet, "/ltpx/tools", nil)
	if code != http.StatusOK {
		LoggerGeneral.Warn("CrystalAstral", "LTPX 工具链构建异常（状态码 %d）", code)
		return ltpx.Status{}
	}

	var resp struct {
		Tool  LTPXRemoteToolDef   `json:"tool"`
		Tools []LTPXRemoteToolDef `json:"tools"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		LoggerGeneral.Warn("CrystalAstral", "LTPX 工具链解析失败: %v", err)
		return ltpx.Status{}
	}
	if resp.Tool.Name == "" {
		return ltpx.Status{}
	}

	// 应用清单随状态一并透传：月华据此把工具名写进 tool 参数枚举，
	// 并保存「工具名 → 描述」映射供 details 查询按名回查
	tools := make([]ltpx.ToolRef, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		if t.Name == "" {
			continue
		}
		tools = append(tools, ltpx.ToolRef{Name: t.Name, Description: t.Description})
	}

	return ltpx.Status{
		Tool: &ltpx.ToolDef{
			Name:        resp.Tool.Name,
			Description: resp.Tool.Description,
			// 参数 schema 必须随定义透传：缺失会让模型收到 parameters=null 的函数定义，
			// 无从得知 {details, tool, instruction} 的参数语义
			Parameters: resp.Tool.Parameters,
		},
		Tools: tools,
	}
}

// Submit 异步受理工具调用，返回 request_id（原 /ltpx/call）。
// 内置智能体在独立 goroutine 中执行，前端包工具经集线器广播给前端页面执行，
// 两者均在完成后写入待定调用的结果通道。
func (b *Broker) Submit(tool string, args map[string]any) (string, error) {
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal(LTPXRemoteCallRequest{Tool: tool, Arguments: args})
	if err != nil {
		return "", fmt.Errorf("序列化工具调用失败: %w", err)
	}

	code, body := invokeEndpoint(LTPRemoteCallHandler, http.MethodPost, "/ltpx/call", payload)
	if code != http.StatusOK {
		return "", fmt.Errorf("受理工具 %s 失败（状态码 %d）: %s", tool, code, string(body))
	}

	var resp LTPXRemoteCallResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("解析工具 %s 受理响应失败: %w", tool, err)
	}
	if resp.Pending && resp.RequestID != "" {
		return resp.RequestID, nil
	}
	if !resp.Success {
		return "", fmt.Errorf("工具 %s 受理被拒: %s", tool, ltpxFirstNonEmpty(resp.Error, "未知原因"))
	}
	return "", fmt.Errorf("工具 %s 未返回 request_id", tool)
}

// Await 等待待定调用的执行结果（进程内结果通道，取代原 HTTP 心跳轮询）。
// 取走结果后清理登记；上下文超时即放弃（不再有「连续失败判离线」语义）。
func (b *Broker) Await(ctx context.Context, requestID string) (ltpx.Result, error) {
	ltpPendingMutex.Lock()
	call, exists := ltpPendingCalls[requestID]
	ltpPendingMutex.Unlock()

	if !exists {
		return ltpx.Result{}, fmt.Errorf("待定调用 %s 不存在（可能已取走）", requestID)
	}

	select {
	case resp := <-call.done:
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		LoggerGeneral.Info("CrystalAstral", "LTPX 工具 %s (request_id=%s) 取回结果: success=%v", call.toolName, requestID, resp.Success)
		return ltpx.Result{Success: resp.Success, Text: resp.Text, Error: resp.Error}, nil
	case <-ctx.Done():
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		return ltpx.Result{}, fmt.Errorf("等待工具 %s 执行回执超时: %w", call.toolName, ctx.Err())
	}
}

// Call 提交并同步等待工具执行结果
func (b *Broker) Call(ctx context.Context, tool string, args map[string]any) (ltpx.Result, error) {
	requestID, err := b.Submit(tool, args)
	if err != nil {
		return ltpx.Result{}, err
	}
	return b.Await(ctx, requestID)
}

// Event 推送「xx事件发生前」负载并同步取回 LTP9 插件处理结果（原 /ltpx/event）。
// 无插件订阅 / 未改写 / 未回传时 returned=false、modified=false，调用方回退本地默认处理。
func (b *Broker) Event(topic string, payload any) ltpx.EventResult {
	payloadBody, err := json.Marshal(LTPXRemoteEventRequest{Topic: topic, Payload: payload})
	if err != nil {
		LoggerGeneral.Warn("CrystalAstral", "LTPX 事件 %s 序列化失败: %v", topic, err)
		return ltpx.EventResult{}
	}

	code, body := invokeEndpoint(LTPRemoteEventHandler, http.MethodPost, "/ltpx/event", payloadBody)
	if code != http.StatusOK {
		return ltpx.EventResult{}
	}

	var resp struct {
		OK       bool `json:"ok"`
		Modified bool `json:"modified"`
		Data     any  `json:"data"`
		Returned bool `json:"returned"`
		Return   any  `json:"return"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ltpx.EventResult{}
	}

	return ltpx.EventResult{
		Modified: resp.Modified,
		Data:     resp.Data,
		Returned: resp.Returned,
		Return:   resp.Return,
	}
}

// ltpxFirstNonEmpty 返回首个非空字符串（错误信息兜底）
func ltpxFirstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
