package CrystalPersonality

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 覆盖历史缺陷：go:embed 指令与读取前缀错配。
//
// 三个注入型智能体脚本先前放在琉璃桌面的静态资源目录里（指令 `go:embed web/*`），
// 而 handler 却按 "assets/xxx" 取，于是 /shared-input.js 与 /mini-ltp-agent.js 一律
// 500 —— 琉璃前端的注入链路永久失效，Mini-LTP / Self-LTP 包只打开页面、任务不执行
// 也不回执，且前端只看到 HTTP 500 无具体原因。
//
// 该错配 go 编译器与 tsc 都不会报错，只有真跑一遍 handler 才暴露。现在脚本归
// agents 包内嵌（`go:embed *.js`，见 agents/agents.go），handler 统一经
// agents.ReadScript 取用，本条仍必须由测试守住"下发得出去、且下发的是对的文件"。
func TestInjectedAgentAssetsAreServable(t *testing.T) {
	cases := []struct {
		name        string
		path        string
		handler     http.HandlerFunc
		mustContain string // 脚本内必须出现的关键标识（防止下发了错误文件）
	}{
		{"Mini-LTP 智能体", "/mini-ltp-agent.js", MiniLTPAgentHandler, "enableAtoA"},
		{"Self-LTP 智能体", "/self-ltp-agent.js", SelfLTPAgentHandler, "SelfLTPAgent"},
		{"SharedInput 共享模块", "/shared-input.js", SharedInputHandler, "SharedInput"},
	}

	served := make(map[string]bool, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("%s 下发失败: status=%d body=%q（检查 agents 包的 go:embed *.js 与 ReadScript 的前缀是否一致）",
					tc.path, rec.Code, strings.TrimSpace(rec.Body.String()))
			}
			body := rec.Body.String()
			if len(body) == 0 {
				t.Fatalf("%s 下发内容为空", tc.path)
			}
			if !strings.Contains(body, tc.mustContain) {
				t.Fatalf("%s 下发内容缺少关键标识 %q（可能下发了错误的资源）", tc.path, tc.mustContain)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
				t.Fatalf("%s Content-Type 异常: %q", tc.path, ct)
			}
			// 智能体脚本必须禁缓存，否则浏览器会复用旧脚本、注入逻辑静默失效
			if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Fatalf("%s 缺少 no-store 缓存控制: %q", tc.path, cc)
			}
			served[tc.path] = true
		})
	}

	// 成对下发约束：Mini-LTP / Self-LTP 依赖 SharedInput 提供的页面操作原语，
	// 三者必须都能取到，否则智能体会在运行时报"模块未加载"。
	for _, path := range []string{"/shared-input.js", "/mini-ltp-agent.js", "/self-ltp-agent.js"} {
		if !served[path] {
			t.Fatalf("%s 未能成功下发（成对下发约束被破坏）", path)
		}
	}
}

// 三个下发型 handler 只接受 GET：其他方法必须拒绝，
// 避免被当成通用资源端点误用。
func TestInjectedAgentAssetsRejectNonGet(t *testing.T) {
	cases := []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/mini-ltp-agent.js", MiniLTPAgentHandler},
		{"/self-ltp-agent.js", SelfLTPAgentHandler},
		{"/shared-input.js", SharedInputHandler},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		tc.handler(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s 对 POST 的响应应为 405，实际 %d", tc.path, rec.Code)
		}
	}
}
