package resourcehub

// util_test.go 参数形状与模式数组的兼容性。

import (
	"reflect"
	"testing"
)

func TestStringListArgAcceptsStringAndArray(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want []string
	}{
		{"数组", map[string]any{"type": []any{"handle", "memory"}}, []string{"handle", "memory"}},
		{"字符串数组", map[string]any{"type": []string{"handle"}}, []string{"handle"}},
		{"单值字符串（兼容旧调用）", map[string]any{"type": "handle"}, []string{"handle"}},
		{"空数组", map[string]any{"type": []any{}}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stringListArg(tc.args, "type"); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("stringListArg() = %v，期望 %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeQueryTypes(t *testing.T) {
	got := normalizeQueryTypes([]string{"Handle", " handle ", "", "MEMORY", "handle"})
	want := []string{"handle", "memory"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeQueryTypes() = %v，期望 %v", got, want)
	}
}
