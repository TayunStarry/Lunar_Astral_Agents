package QQMessenger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

// TestPlanSendParsesJSON 钉住「需求 → 结构化投递计划」：提示词可读、JSON 可解析。
func TestPlanSendParsesJSON(t *testing.T) {
	planJSON := `{"target_type":"group","target":"测试群","text":"晚安","images":["@I.001"],"files":[],"reason":""}`
	response, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": planJSON}}},
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()

	origURL, origModel, origKey := config.AgentMultimodalURL, config.AgentMultimodalModel, config.AgentMultimodalKey
	modelOnce = sync.Once{}
	modelName, modelURL, modelKey = "", "", ""
	config.AgentMultimodalURL = server.URL
	config.AgentMultimodalModel = "test-model"
	config.AgentMultimodalKey = ""
	t.Cleanup(func() {
		config.AgentMultimodalURL, config.AgentMultimodalModel, config.AgentMultimodalKey = origURL, origModel, origKey
		modelOnce = sync.Once{}
		modelName, modelURL, modelKey = "", "", ""
	})

	groups, friends := testDirectory()
	plan, err := planSend("把 @I.001 和这句话发到群里", groups, friends)
	if err != nil {
		t.Fatalf("解析投递计划失败: %v", err)
	}
	if plan.TargetType != "group" || plan.Target != "测试群" || plan.Text != "晚安" || len(plan.Images) != 1 {
		t.Fatalf("投递计划解析不符: %#v", plan)
	}
}
