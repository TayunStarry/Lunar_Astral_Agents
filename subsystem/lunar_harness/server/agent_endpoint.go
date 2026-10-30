package server

// agent_endpoint.go 外部智能体接入端点：/agent 系列。
//
// 近似 OpenAI v1，但有两条硬约束：
//   - 只提供非流式输出；
//   - 不接受调用方指定模型名（推理固定 primary_reasoning，嵌入固定 text_embedding）。
//
// 推理分支把请求交给后勤系统的响应者智能体排队处理：只有当 Tick 体系空闲时
// 才会被消费，因此天然与 Go 智能体的模型调用互斥，不会抢占正在推理的智能体。
// 嵌入分支是纯转发（不涉及生成式推理，直接打到 llama.cpp）。

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

const (
	// agentWaitTimeout 前端等待本次推理的最长时间
	agentWaitTimeout = 10 * time.Minute
	// agentMaxBodyBytes 请求体上限
	agentMaxBodyBytes = 32 << 20
	// agentModule 日志模块名
	agentModule = "AgentEndpoint"
)

// agentHTTPClient 嵌入转发专用的 HTTP 客户端
var agentHTTPClient = &http.Client{Timeout: 2 * time.Minute}

// AgentChatHandler 外部多模态推理：入队 → 等后勤 Tick 处理 → 非流式返回。
func AgentChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := readAgentPayload(r)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages, ok := payload["messages"]
	if !ok || len(messages) == 0 {
		writeAgentError(w, http.StatusBadRequest, "messages 不能为空")
		return
	}
	params := payload
	delete(params, "messages")
	req := &Responder.Request{
		Messages: messages,
		Params:   params,
		Reply:    make(chan Responder.Result, 1),
	}
	if err := Responder.Enqueue(req); err != nil {
		logger.Warn(agentModule, "外部推理入队失败: %v", err)
		writeAgentError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	logger.Info(agentModule, "外部推理已入队（排队 %d 条）", Responder.Pending())

	select {
	case result := <-req.Reply:
		if result.Err != nil {
			writeAgentError(w, http.StatusBadGateway, result.Err.Error())
			return
		}
		writeAgentChat(w, result)
	case <-time.After(agentWaitTimeout):
		writeAgentError(w, http.StatusGatewayTimeout, "等待推理超时")
	case <-r.Context().Done():
		logger.Warn(agentModule, "前端在等待推理时断开连接")
	}
}

// AgentEmbeddingHandler 外部文本嵌入：固定模型名转发到 llama.cpp。
func AgentEmbeddingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	payload, err := readAgentPayload(r)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := payload["input"]; !ok {
		writeAgentError(w, http.StatusBadRequest, "input 不能为空")
		return
	}
	payload["model"], _ = json.Marshal(config.AgentEmbeddingModel)
	delete(payload, "stream")
	body, err := json.Marshal(payload)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, "请求序列化失败")
		return
	}
	url := strings.TrimRight(config.AgentEmbeddingURL, "/") + "/embeddings"
	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		writeAgentError(w, http.StatusInternalServerError, "构造嵌入请求失败")
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	resp, err := agentHTTPClient.Do(proxyReq)
	if err != nil {
		logger.Warn(agentModule, "嵌入请求失败: %v", err)
		writeAgentError(w, http.StatusBadGateway, "嵌入服务不可用")
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "读取嵌入响应失败")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(raw)
}

// readAgentPayload 读取并解析 JSON 对象请求体（字段值原样保留）。
func readAgentPayload(r *http.Request) (map[string]json.RawMessage, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, agentMaxBodyBytes))
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}
	return payload, nil
}

// writeAgentChat 输出与 OpenAI v1 兼容的非流式响应。
// tool_calls 与 finish_reason 必须原样带出：外部智能体靠它判断是否继续工具循环，
// 少了 tool_calls 会让带工具链的智能体在输出一句引子后直接停下。
func writeAgentChat(w http.ResponseWriter, result Responder.Result) {
	finishReason := strings.TrimSpace(result.FinishReason)
	if finishReason == "" {
		finishReason = "stop"
	}
	if len(result.ToolCalls) > 0 && finishReason == "stop" {
		finishReason = "tool_calls"
	}
	message := AgentChatMessage{
		Role:             "assistant",
		Content:          result.Content,
		ReasoningContent: result.Reasoning,
	}
	if len(result.ToolCalls) > 0 {
		message.ToolCalls = result.ToolCalls
	}
	response := AgentChatResponse{
		Object: "chat.completion",
		Model:  config.AgentMultimodalModel,
		Choices: []AgentChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// writeAgentError 输出错误响应。
func writeAgentError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(AgentChatResponse{
		Object: "error",
		Error:  &AgentChatError{Message: message},
	})
}
