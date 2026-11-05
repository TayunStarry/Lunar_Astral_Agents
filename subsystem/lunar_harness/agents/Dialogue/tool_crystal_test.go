package Dialogue

// tool_crystal_test.go 钉住 use_the_program 的只读查询路径：details=true 只返回职能说明，
// 不发起远端工具调用，也不改变应用清单。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// TestUseTheProgramDetailsHasNoSideEffect details=true 只返回应用的职能说明。
func TestUseTheProgramDetailsHasNoSideEffect(t *testing.T) {
	remoteMu.Lock()
	previous := remoteTools
	remoteTools = []remoteToolRef{{Name: "chart_render", Description: "图表渲染工坊：按自然语言生成 ECharts / Mermaid 图表"}}
	remoteMu.Unlock()
	t.Cleanup(func() {
		remoteMu.Lock()
		remoteTools = previous
		remoteMu.Unlock()
	})

	decoded := decodeCrystalOutput(t, ExecuteUseTheProgram(map[string]any{
		"type":    "chart_render",
		"details": true,
		"handle":  []any{},
	}))
	if decoded.Error != "" {
		t.Fatalf("details 查询不应报错: %s", decoded.Error)
	}
	if !strings.Contains(decoded.Result, "图表渲染工坊") {
		t.Fatalf("details 应返回应用职能描述，实际 %q", decoded.Result)
	}
	if len(decoded.Handle) != 0 {
		t.Fatalf("details 查询不应涉及句柄，实际 %#v", decoded.Handle)
	}
}

// decodeCrystalOutput 解析统一输出原语。
func decodeCrystalOutput(t *testing.T, result ToolResult) agentdispatch.Result {
	t.Helper()
	var decoded agentdispatch.Result
	if err := json.Unmarshal([]byte(result.Text), &decoded); err != nil {
		t.Fatalf("工具输出不是统一原语: %v（%q）", err, result.Text)
	}
	return decoded
}
