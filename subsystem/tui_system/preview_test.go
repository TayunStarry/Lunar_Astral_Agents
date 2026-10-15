package TuiSystem

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// 渲染预览：以 `go test -run TestRenderPreview -v ./subsystem/tui_system/` 查看各屏在固定
// 终端尺寸下的实际排版（含中文对齐、框线、滚动条），供无 TTY 环境下人工核对布局。
// t.Logf 输出只在 -v 时可见，默认测试运行不受影响。

// previewScreens 预览用的终端尺寸
var previewScreens = []struct {
	name string
	w, h int
}{
	{"常规 100×30", 100, 30},
	{"紧凑 80×24", 80, 24},
}

// TestRenderPreview 全屏预览
func TestRenderPreview(t *testing.T) {
	seedAgentTurns(3)
	cases := screenCases()
	for _, size := range previewScreens {
		for _, tc := range cases {
			m := tc.build(size.w, size.h)
			t.Logf("\n===== %s（%s）=====\n%s", tc.name, size.name, ansi.Strip(m.View()))
		}
	}
}
