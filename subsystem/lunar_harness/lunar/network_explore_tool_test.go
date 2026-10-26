package lunar

// network_explore_tool_test.go 钉住「Lunar Matrix 仅在实际可用时暴露」的条件枚举逻辑。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkExploreExposesLunarMatrixOnlyWhenReady(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	t.Setenv("LUNAR_MATRIX_CONFIG", missing)
	if enumContains(t, networkExploreEnum(t), "lunar_matrix") {
		t.Fatal("配置缺失时不应暴露 lunar_matrix")
	}

	disabled := writeMatrixConfig(t, map[string]any{"enabled": false, "base_url": "https://example.com"})
	t.Setenv("LUNAR_MATRIX_CONFIG", disabled)
	if enumContains(t, networkExploreEnum(t), "lunar_matrix") {
		t.Fatal("开关关闭时不应暴露 lunar_matrix")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": map[string]any{
				"enabled": true, "api_version": "v1", "enroll_mode": "open",
				"poll_interval_seconds": 60, "round_ttl_seconds": 600,
				"server_time": "2026-10-25T12:00:00+08:00",
			},
			"meta": map[string]any{
				"request_id": "test", "server_time": "2026-10-25T12:00:00+08:00", "api_version": "v1",
			},
		})
	}))
	defer server.Close()

	enabled := writeMatrixConfig(t, map[string]any{
		"enabled": true, "base_url": server.URL,
		"health_timeout_seconds": 1, "health_cache_seconds": 30,
	})
	t.Setenv("LUNAR_MATRIX_CONFIG", enabled)
	enum := networkExploreEnum(t)
	if !enumContains(t, enum, "lunar_matrix") {
		t.Fatal("配置开启且论坛可达时应暴露 lunar_matrix")
	}
	for _, want := range []string{"web_search", "bilibil_viewer"} {
		if !enumContains(t, enum, want) {
			t.Fatalf("基础能力 %s 不应被移除", want)
		}
	}
}

func networkExploreEnum(t *testing.T) []string {
	t.Helper()
	tools := networkExploreToolCalls()
	if len(tools) != 1 {
		t.Fatalf("聚合工具数量应为 1，实际 %d", len(tools))
	}
	params, ok := tools[0].Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("工具参数 Schema 类型异常: %T", tools[0].Function.Parameters)
	}
	properties := params["properties"].(map[string]any)
	toolProp := properties["tool"].(map[string]any)
	enum, ok := toolProp["enum"].([]string)
	if !ok {
		t.Fatalf("tool 枚举类型异常: %T", toolProp["enum"])
	}
	return enum
}

func enumContains(t *testing.T, values []string, target string) bool {
	t.Helper()
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeMatrixConfig(t *testing.T, cfg map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lunar_matrix.json")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
