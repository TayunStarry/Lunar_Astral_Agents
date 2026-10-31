package CrystalPersonality

import "testing"

// 聚合工具分工的契约：本端只返回聚合工具名与应用清单。
//
// 参数 schema（details/tool/instruction 与 tool 枚举）由月华按清单统一构建，
// 本端不再携带，避免同一份 schema 在两端各维护一遍后漂移。
func TestUseTheProgramReturnsToolNameAndRefs(t *testing.T) {
	status := NewBroker().Status()
	if status.Tool == nil {
		t.Fatal("工具链状态缺少聚合工具（tool）")
	}
	if status.Tool.Name != integratedToolsName {
		t.Fatalf("聚合工具名应为 %s，实得 %q", integratedToolsName, status.Tool.Name)
	}
	// 内置桌面操纵已不再暴露，未安装扩展包时清单为空也是合法状态；
	// 这里只校验清单里没有空名称。
	for _, ref := range status.Tools {
		if ref.Name == "" {
			t.Fatal("应用清单存在空名称")
		}
	}
}
