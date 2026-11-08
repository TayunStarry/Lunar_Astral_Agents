package MemoryTagger

// agent_test.go 标签智能体的请求契约：关闭思考、保留输出预算、解析 JSON 数组。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

func TestGenerateDisablesThinkingAndParsesTags(t *testing.T) {
	var got struct {
		Model              string         `json:"model"`
		MaxTokens          int            `json:"max_tokens"`
		ChatTemplateKwargs map[string]any `json:"chat_template_kwargs"`
		Stream             bool           `json:"stream"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("解析标签请求失败: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[\"猫\",\"表情\"]"}}]}`))
	}))
	defer server.Close()

	agentbase.SetModelEndpointProvider(func() agentbase.ModelEndpoint {
		return agentbase.ModelEndpoint{URL: server.URL, Model: "test-multimodal"}
	})
	agentbase.ResetModelCache()
	t.Cleanup(func() {
		agentbase.SetModelEndpointProvider(nil)
		agentbase.ResetModelCache()
	})

	tags, err := Generate(context.Background(), "data:image/png;base64,AAAA", true, "auto", "", nil)
	if err != nil {
		t.Fatalf("标签生成失败: %v", err)
	}
	if len(tags) != 2 || tags[0] != "猫" {
		t.Fatalf("标签结果不正确: %v", tags)
	}
	if enabled, ok := got.ChatTemplateKwargs["enable_thinking"]; !ok || enabled != false {
		t.Fatalf("标签请求未关闭思考: %+v", got.ChatTemplateKwargs)
	}
	if got.MaxTokens < maxTokens {
		t.Fatalf("标签请求 max_tokens 过小，推理可能耗尽输出: %d", got.MaxTokens)
	}
	if !got.Stream {
		t.Fatal("标签请求应使用流式调用，以便推理实况可见")
	}
}
