package agentbase

// stream_test.go 统一模型调用的端到端测试：SSE 聚合、工具调用与包装响应。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStreamAggregatesSSE 思考 / 正文 / 工具调用必须聚合为同一份 Reply。
func TestStreamAggregatesSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []map[string]any{
			{"choices": []any{map[string]any{"delta": map[string]any{"reasoning_content": "先想一下"}}}},
			{"choices": []any{map[string]any{"delta": map[string]any{"content": "你好"}}}},
			{"choices": []any{map[string]any{"delta": map[string]any{"content": "，世界"}}}},
			{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{
				map[string]any{"index": 0, "id": "call_1", "type": "function",
					"function": map[string]any{"name": "ping", "arguments": `{"x":1}`}},
			}}}}},
		}
		for _, chunk := range chunks {
			raw, _ := json.Marshal(chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	restoreModelEndpoint(t, server.URL, "test-model")
	reply, err := Stream(t.Context(), StreamRequest{
		Role:     "测试智能体",
		Messages: []Message{{Role: "user", Content: "你好"}},
	})
	if err != nil {
		t.Fatalf("统一流式调用失败: %v", err)
	}
	if reply.Text != "你好，世界" {
		t.Fatalf("正文聚合错误: %q", reply.Text)
	}
	if reply.Reasoning != "先想一下" {
		t.Fatalf("思考聚合错误: %q", reply.Reasoning)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Function.Name != "ping" {
		t.Fatalf("工具调用聚合错误: %#v", reply.ToolCalls)
	}
}

// TestStreamParsesWrappedResponse 兼容 {success,data} 代理包装的非流式响应。
func TestStreamParsesWrappedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"choices": []any{map[string]any{
					"message": map[string]any{"content": "包装响应"},
				}},
			},
		})
	}))
	defer server.Close()

	restoreModelEndpoint(t, server.URL, "test-model")
	reply, err := Stream(t.Context(), StreamRequest{
		Role:      "测试智能体",
		Messages:  []Message{{Role: "user", Content: "你好"}},
		NonStream: true,
	})
	if err != nil {
		t.Fatalf("包装响应解析失败: %v", err)
	}
	if reply.Text != "包装响应" {
		t.Fatalf("正文解析错误: %q", reply.Text)
	}
}

// restoreModelEndpoint 临时注入假端点提供者并清理缓存。
func restoreModelEndpoint(t *testing.T, url, model string) {
	t.Helper()
	SetModelEndpointProvider(func() ModelEndpoint {
		return ModelEndpoint{URL: url, Model: model}
	})
	ResetModelCache()
	t.Cleanup(func() {
		SetModelEndpointProvider(nil)
		ResetModelCache()
	})
}
