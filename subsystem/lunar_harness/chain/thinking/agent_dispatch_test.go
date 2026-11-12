package thinking

// agent_dispatch_test.go 钉住智能体聚合工具的统一输入输出契约。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// TestTechniqueDetailsHasNoSideEffect details=true 只返回职能描述，不调用子智能体。
func TestTechniqueDetailsHasNoSideEffect(t *testing.T) {
	result := Dialogue.ExecuteCreativeWork(map[string]any{
		"type":    Dialogue.TechniquePainter,
		"details": true,
		"handle":  []any{},
	})
	decoded := decodeToolOutput(t, result)
	if decoded.Error != "" {
		t.Fatalf("details 查询不应报错: %s", decoded.Error)
	}
	if !strings.Contains(decoded.Result, "图像绘制") {
		t.Fatalf("details 应返回图像绘制职能描述，实际 %q", decoded.Result)
	}
	if len(decoded.Handle) != 0 {
		t.Fatalf("details 查询不应涉及句柄，实际 %#v", decoded.Handle)
	}
}

// TestTechniqueRejectsNonUnifiedInput 非统一原语（旧 reference_tag 等）必须返回格式错误。
func TestTechniqueRejectsNonUnifiedInput(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"旧字段 reference_tag", map[string]any{"type": Dialogue.TechniquePainter, "instruction": "画一张图", "reference_tag": "@I.003"}, "不支持的字段"},
		{"缺 type", map[string]any{"instruction": "画一张图"}, "type 不能为空"},
		{"缺 instruction", map[string]any{"type": Dialogue.TechniquePainter, "handle": []any{}}, "instruction 不能为空"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decoded := decodeToolOutput(t, Dialogue.ExecuteCreativeWork(tc.args))
			if !strings.Contains(decoded.Error, tc.want) {
				t.Fatalf("错误应包含 %q，实际 %q", tc.want, decoded.Error)
			}
			if decoded.Handle == nil || len(decoded.Handle) != 0 {
				t.Fatalf("格式错误时 handle 必须为空数组，实际 %#v", decoded.Handle)
			}
		})
	}
}

// decodeToolOutput 解析统一输出原语，并校验三段字段的存在性。
func decodeToolOutput(t *testing.T, result ToolResult) agentdispatch.Result {
	t.Helper()
	var decoded agentdispatch.Result
	if err := json.Unmarshal([]byte(result.Text), &decoded); err != nil {
		t.Fatalf("工具输出不是统一原语: %v（%q）", err, result.Text)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(result.Text), &raw); err != nil {
		t.Fatalf("工具输出不是合法 JSON: %v", err)
	}
	for _, key := range []string{"result", "handle", "error"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("统一输出缺少字段 %s：%q", key, result.Text)
		}
	}
	if len(raw) != 3 {
		t.Fatalf("统一输出不能有额外字段：%q", result.Text)
	}
	return decoded
}
