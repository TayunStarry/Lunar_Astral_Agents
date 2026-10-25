package crystal

import (
	"encoding/json"
	"strings"
	"testing"
)

// 聚合工具必须把全部应用名写进 tool 参数枚举，并随状态返回结构化清单。
//
// 这条钉的是月华侧的隐式契约：enum 让模型启动时直接看到可选应用名（不再先查清单），
// tools 让 details 能按名回查描述。两端分居 Go/TS，改动其中一端不会有编译期告警，
// 只能靠测试把 JSON 形状固定住。
func TestUseTheProgramExposesToolEnumAndRefs(t *testing.T) {
	status := NewBroker().Status()
	if status.Tool == nil {
		t.Fatal("工具链状态缺少聚合工具（tool）")
	}
	// 内置桌面操纵已不再暴露，未安装扩展包时工具链为空也是合法状态；
	// 这里只校验「枚举与清单一致」的契约，不强制要求至少有一个工具。

	names := make([]string, 0, len(status.Tools))
	for _, ref := range status.Tools {
		if ref.Name == "" {
			t.Fatal("应用清单存在空名称")
		}
		names = append(names, ref.Name)
	}

	raw, err := json.Marshal(status.Tool.Parameters)
	if err != nil {
		t.Fatalf("序列化聚合工具参数失败: %v", err)
	}
	var schema struct {
		Properties struct {
			Details struct {
				Type string `json:"type"`
			} `json:"details"`
			Tool struct {
				Enum []string `json:"enum"`
			} `json:"tool"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("解析聚合工具参数失败: %v", err)
	}
	if schema.Properties.Details.Type != "boolean" {
		t.Fatalf("details 参数应为 boolean，实得 %q", schema.Properties.Details.Type)
	}
	if strings.Join(schema.Properties.Tool.Enum, ",") != strings.Join(names, ",") {
		t.Fatalf("tool 枚举与应用清单不一致: enum=%v tools=%v", schema.Properties.Tool.Enum, names)
	}
}
