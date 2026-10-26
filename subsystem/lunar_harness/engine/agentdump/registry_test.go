package agentdump

// registry_test.go 钉住导出注册表的基本契约：注册、列举、JSON 导出与未知目标报错。

import (
	"strings"
	"testing"
)

func TestRegisterListAndExport(t *testing.T) {
	Register(Provider{
		ID:    "test_target",
		Label: "测试目标",
		Group: "测试",
		Export: func(includeImages bool) (any, error) {
			return map[string]any{"kind": "test", "message_count": 2, "include_images": includeImages}, nil
		},
	})

	found := false
	for _, provider := range List() {
		if provider.ID == "test_target" {
			found = true
		}
	}
	if !found {
		t.Fatal("注册后 List 应包含 test_target")
	}

	content, err := Export("test_target", true)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if !strings.Contains(content, `"kind": "test"`) || !strings.Contains(content, `"message_count": 2`) {
		t.Fatalf("导出内容缺少字段:\n%s", content)
	}

	if _, err := Export("missing_target", false); err == nil {
		t.Fatal("未知目标应返回错误")
	}
}
