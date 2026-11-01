package Creator

// fetch_resources_test.go 创作体系统一资源工具的入参校验。

import (
	"context"
	"strings"
	"testing"
)

func TestCreatorFetchResourcesValidation(t *testing.T) {
	s := &session{}
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"非法 type", map[string]any{"type": "bad"}, "type 只能取 memory / handle / list"},
		{"memory 缺 query", map[string]any{"type": "memory"}, "memory 模式需要 query 关键词"},
		{"handle 缺 query", map[string]any{"type": "handle"}, "handle 模式需要 query 句柄数组"},
		{"list 类型码非法", map[string]any{"type": "list", "query": []any{"X"}}, "类型码无效"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := s.fetchResources(context.Background(), tc.args)
			if !strings.Contains(out.Summary, tc.want) {
				t.Fatalf("提示应包含 %q，实际 %q", tc.want, out.Summary)
			}
		})
	}
}
