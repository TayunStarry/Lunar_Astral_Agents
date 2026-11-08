package Responder

// model_test.go 外部推理的工具链路：/agent/chat/completions 收到的 tools
// 必须真正下发到模型，且模型流式返回的工具调用必须完整回传。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func TestCompleteOnceForwardsTools(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("解析模型请求失败: %v", err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function",` +
				`"function":{"name":"execute_operations","arguments":"{\"operations\":[{\"action\":\"click\"}]}"}}]}}]}` + "\n\n" +
				"data: [DONE]\n\n"))
	}))
	defer server.Close()
	restoreResponderEndpoint(t, server.URL)

	tools := json.RawMessage(`[{"type":"function","function":{"name":"execute_operations",` +
		`"description":"执行操作队列","parameters":{"type":"object",` +
		`"properties":{"operations":{"type":"array","items":{"type":"object"}}},"required":["operations"]}}}]`)
	req := &Request{
		Messages: json.RawMessage(`[{"role":"user","content":"点击按钮"}]`),
		Params:   map[string]json.RawMessage{"tools": tools},
	}

	result, err := completeOnce(t.Context(), req)
	if err != nil {
		t.Fatalf("推理失败: %v", err)
	}
	if _, ok := requestBody["tools"]; !ok {
		t.Fatalf("工具定义未下发到模型，请求体: %v", requestBody)
	}
	if choice, _ := requestBody["tool_choice"].(string); choice != "auto" {
		t.Fatalf("工具选择模式错误: %v", requestBody["tool_choice"])
	}
	calls := parseToolCalls(result.ToolCalls)
	if len(calls) != 1 || calls[0].Function.Name != "execute_operations" {
		t.Fatalf("工具调用未完整回传: %s", string(result.ToolCalls))
	}
}

// restoreResponderEndpoint 临时注入假模型端点并清理缓存。
func restoreResponderEndpoint(t *testing.T, url string) {
	t.Helper()
	agentbase.SetModelEndpointProvider(func() agentbase.ModelEndpoint {
		return agentbase.ModelEndpoint{URL: url, Model: "test-model"}
	})
	agentbase.ResetModelCache()
	t.Cleanup(func() {
		agentbase.SetModelEndpointProvider(nil)
		agentbase.ResetModelCache()
	})
}
