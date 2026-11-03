package agentdispatch

// agentdispatch_test.go 钉住统一输入解析与输出序列化的边界。

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseValidCall(t *testing.T) {
	call, err := Parse(map[string]any{
		"type":        " painter ",
		"instruction": "画一张赛博朋克自画像",
		"handle":      []any{"@I.003", " "},
		"details":     false,
	})
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if call.Type != "painter" {
		t.Fatalf("type 应去掉首尾空白，实际 %q", call.Type)
	}
	if len(call.Handle) != 1 || call.Handle[0] != "@I.003" {
		t.Fatalf("handle 应保留有效句柄，实际 %#v", call.Handle)
	}
	if call.Details {
		t.Fatal("details 应为 false")
	}
}

func TestParseRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"未知字段", map[string]any{"type": "painter", "tool": "painter"}, "不支持的字段"},
		{"缺 type", map[string]any{"instruction": "x"}, "type 不能为空"},
		{"type 类型错误", map[string]any{"type": 1}, "type 必须是字符串"},
		{"instruction 类型错误", map[string]any{"type": "painter", "instruction": 1}, "instruction 必须是字符串"},
		{"handle 类型错误", map[string]any{"type": "painter", "handle": "@I.003"}, "handle 必须是字符串数组"},
		{"handle 元素类型错误", map[string]any{"type": "painter", "handle": []any{1}}, "每一项都必须是字符串"},
		{"details 类型错误", map[string]any{"type": "painter", "details": "true"}, "details 必须是布尔值"},
		{"控制字符", map[string]any{"type": "painter", "instruction": "画图\x00"}, "控制字符"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("错误应包含 %q，实际 %v", tc.want, err)
			}
		})
	}
}

func TestResultJSONHasOnlyThreeFields(t *testing.T) {
	raw := Success("完成", nil).JSON()
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	if len(decoded) != 3 {
		t.Fatalf("输出字段必须恰好为三个，实际 %v", decoded)
	}
	var handle []string
	if err := json.Unmarshal(decoded["handle"], &handle); err != nil {
		t.Fatalf("handle 必须是数组: %v", err)
	}
	if handle == nil || len(handle) != 0 {
		t.Fatalf("无句柄时必须输出空数组，实际 %s", string(decoded["handle"]))
	}
	var errText string
	if err := json.Unmarshal(decoded["error"], &errText); err != nil || errText != "" {
		t.Fatalf("无错误时 error 必须是空字符串，实际 %s", string(decoded["error"]))
	}
}

func TestSchemaCarriesUnifiedFields(t *testing.T) {
	schema := Schema("painter=绘画", []string{"painter"}, "任务描述", "")
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties 类型异常: %T", schema["properties"])
	}
	for _, field := range []string{FieldType, FieldInstruction, FieldHandle, FieldDetails} {
		if _, exists := properties[field]; !exists {
			t.Fatalf("schema 缺少字段 %s", field)
		}
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 4 {
		t.Fatalf("schema 必须要求四个统一字段，实际 %#v", schema["required"])
	}
}
