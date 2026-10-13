package BrowserClient

import (
	"strings"
	"testing"
)

// TestPathOf 校验主页面路径提取
func TestPathOf(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:36789/desktop/": "/desktop/",
		"http://127.0.0.1:36789/":         "/",
		"http://127.0.0.1:36789":          "/",
		"":                                "/",
	}
	for in, want := range cases {
		if got := pathOf(in); got != want {
			t.Errorf("pathOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestReturnButtonGatesByMainPath 注入脚本必须按主页面实际路径判断，
// 不得硬编码根路径 —— 否则主页面不在根路径时（如琉璃桌面 /desktop/）
// 会在主页面自身误显示「返回主页面」按钮。
func TestReturnButtonGatesByMainPath(t *testing.T) {
	script := buildReturnButtonJS("http://127.0.0.1:36789/desktop/")

	if !strings.Contains(script, `"/desktop/"`) {
		t.Fatal("注入脚本未携带主页面路径（应从 mainURL 派生）")
	}
	if strings.Contains(script, "p === '/'") {
		t.Fatal("注入脚本仍在硬编码根路径判断")
	}
	if !strings.Contains(script, "norm(window.location.pathname) === norm(mainPath)") {
		t.Fatal("注入脚本缺少主页面路径比较")
	}
	// 非主页面仍需显示按钮
	if !strings.Contains(script, "btn.id = 'crystal-return-home'") {
		t.Fatal("注入脚本缺少按钮创建逻辑")
	}
}