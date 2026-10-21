package lunarloop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"LunarSubsystem/GeneralConfig"
)

// setAnimationActions 替换动画列表缓存（测试结束后由调用方清理）
func setAnimationActions(names ...string) {
	animCache.Lock()
	defer animCache.Unlock()
	actions := make([]ActionDefinition, 0, len(names))
	for _, name := range names {
		actions = append(actions, ActionDefinition{Name: name})
	}
	animCache.Actions = actions
	animCache.UpdatedAt = 1
}

func resetActionMatchState() {
	animCache.Lock()
	animCache.Actions = nil
	animCache.UpdatedAt = 0
	animCache.Unlock()
	actionVectors = actionVectorIndex{}
}

// startActionEmbeddingServer 假嵌入服务：按关键词映射 4 维向量。
func startActionEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type item struct {
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []item `json:"data"`
		}{}
		for _, text := range req.Input {
			vec := []float32{0, 0, 0, 1}
			switch {
			case strings.Contains(text, "歪"), strings.Contains(text, "晃"):
				vec = []float32{0, 1, 0, 0}
			case strings.Contains(text, "挥"), strings.Contains(text, "手"):
				vec = []float32{0, 0, 1, 0}
			case strings.Contains(text, "秋千"), strings.Contains(text, "花绳"):
				vec = []float32{1, 0, 0, 0}
			}
			resp.Data = append(resp.Data, item{Embedding: vec})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestMatchActionNameUsesAnimationCache 覆盖：动作匹配只读取 animation_list 缓存，
// 用进程内向量索引选出语义最接近的动作名，不往记忆库写任何东西。
func TestMatchActionNameUsesAnimationCache(t *testing.T) {
	server := startActionEmbeddingServer(t)
	defer server.Close()

	origURL, origModel := *GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel
	*GeneralConfig.AgentEmbeddingURL = server.URL + "/v1"
	*GeneralConfig.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel = origURL, origModel
		resetActionMatchState()
	})

	setAnimationActions("荡秋千", "翻花绳", "歪头", "挥手")
	if got := matchActionName("轻轻晃了晃脑袋"); got != "歪头" {
		t.Fatalf("语义匹配应选「歪头」，实得 %q", got)
	}
	if got := matchActionName("挥了挥手"); got != "挥手" {
		t.Fatalf("语义匹配应选「挥手」，实得 %q", got)
	}
}

// TestMatchActionNameFallsBackToLexical 覆盖：嵌入服务不可用时退回字面包含匹配，
// 仍然不访问记忆库。
func TestMatchActionNameFallsBackToLexical(t *testing.T) {
	origURL, origModel := *GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel
	*GeneralConfig.AgentEmbeddingURL = "http://127.0.0.1:1/v1"
	*GeneralConfig.AgentEmbeddingModel = "test-embedding"
	t.Cleanup(func() {
		*GeneralConfig.AgentEmbeddingURL, *GeneralConfig.AgentEmbeddingModel = origURL, origModel
		resetActionMatchState()
	})

	setAnimationActions("挥手", "点头")
	if got := matchActionName("她挥了挥手"); got != "挥手" {
		t.Fatalf("嵌入失败时应按字面包含选「挥手」，实得 %q", got)
	}
	if got := matchActionName("原地发呆"); got != "" {
		t.Fatalf("没有匹配时不应猜动作，实得 %q", got)
	}
}
