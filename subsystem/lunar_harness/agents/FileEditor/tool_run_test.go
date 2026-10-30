package FileEditor

// tool_run_test.go 运行时长解析的验收：首行声明、显式参数优先级与区间夹取。

import (
	"testing"
	"time"
)

func TestRunLimitFromFirstLine(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    time.Duration
	}{
		{"带引擎标识的秒级声明", "//Engine: Lunar Harness | timeout=300s\nmain()", 300 * time.Second},
		{"缺省单位按秒", "//Engine: Lunar Harness | timeout=90\nmain()", 90 * time.Second},
		{"毫秒声明", "// timeout=5000ms\nmain()", 5 * time.Second},
		{"中文单位", "// timeout=2分\nmain()", 2 * time.Minute},
		{"无声明", "//Engine: Lunar Harness\nmain()", 0},
		{"声明写在第二行不生效", "//Engine: Lunar Harness\n// timeout=300s\nmain()", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runLimitFromFirstLine(tc.content); got != tc.want {
				t.Fatalf("首行声明解析错误：期望 %v，实际 %v", tc.want, got)
			}
		})
	}
}

func TestResolveRunLimitPrecedence(t *testing.T) {
	content := "//Engine: Lunar Harness | timeout=300s\nmain()"

	// 显式参数优先于首行声明
	if got := resolveRunLimit(content, 60); got != 60*time.Second {
		t.Fatalf("显式参数应优先：期望 60s，实际 %v", got)
	}
	// 未传参数时采用首行声明
	if got := resolveRunLimit(content, 0); got != 300*time.Second {
		t.Fatalf("未传参数时应采用首行声明：期望 300s，实际 %v", got)
	}
	// 两者都没有时使用默认值
	if got := resolveRunLimit("//Engine: Lunar Harness\nmain()", 0); got != defaultRunLimit {
		t.Fatalf("默认值应为 %v，实际 %v", defaultRunLimit, got)
	}
	// 越界值被夹到允许区间
	if got := resolveRunLimit("", 1); got != minRunLimit {
		t.Fatalf("过短应夹到 %v，实际 %v", minRunLimit, got)
	}
	if got := resolveRunLimit("", 99999); got != maxRunLimit {
		t.Fatalf("过长应夹到 %v，实际 %v", maxRunLimit, got)
	}
}
