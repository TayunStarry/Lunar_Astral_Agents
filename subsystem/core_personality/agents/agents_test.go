package agents

import (
	"strings"
	"testing"
)

// 三个页面注入型智能体脚本必须都能从 agents 包的内嵌资源里取到，且内容非空。
//
// 这是琉璃前端注入链路的地基：SharedInput 承载另外两个智能体共享的页面操作原语，
// 缺失任何一个都会让对应智能体在运行时报"模块未加载"，而前端只能看到一次 500。
func TestReadScriptServesAllInjectedAgents(t *testing.T) {
	cases := []struct {
		name        string
		mustContain string // 防止子目录里的同名文件或错放的脚本被当成智能体下发
	}{
		{"shared_input.js", "SharedInput"},
		{"mini_ltp_agent.js", "enableAtoA"},
		{"self_ltp_agent.js", "SelfLTPAgent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := ReadScript(tc.name)
			if err != nil {
				t.Fatalf("读取内嵌智能体脚本 %s 失败: %v", tc.name, err)
			}
			if len(data) == 0 {
				t.Fatalf("内嵌智能体脚本 %s 为空", tc.name)
			}
			if !strings.Contains(string(data), tc.mustContain) {
				t.Fatalf("内嵌智能体脚本 %s 缺少关键标识 %q（可能取到了错误的文件）", tc.name, tc.mustContain)
			}
		})
	}
}

// ReadScript 必须拒绝空名称，只认 agents/ 本层的脚本，并兼容带 "agents/" 前缀
// 的写法。历史缺陷形态的前缀（web/、assets/）必须报错而不是返回空内容：
// 空内容会让前端拿到 200 空脚本、注入静默失效，比直接 500 难排查得多。
func TestReadScriptPathHandling(t *testing.T) {
	if _, err := ReadScript(""); err == nil {
		t.Fatal("空脚本名称应当报错")
	}

	for _, bad := range []string{
		"web/mini_ltp_agent.js",        // 历史缺陷形态：脚本曾在琉璃前端 web/ 下
		"assets/mini_ltp_agent.js",     // 更早的错配形态
		"prompts/dialogueRole.md",      // 提示词不属于智能体脚本
		"AutoLTP/host.go",              // 子目录（Go 实现）不以脚本形式下发
		"WebLTP/host.go",               //
		"mini_ltp_agent.js/../main.go", // 路径穿越不得成功
		"不存在的智能体.js",                   // 未知脚本
	} {
		if data, err := ReadScript(bad); err == nil {
			t.Fatalf("%q 不应被解析为智能体脚本（取到 %d 字节）", bad, len(data))
		}
	}

	// 带 agents/ 前缀的写法必须与不带前缀等价（避免调用方再猜一次前缀）
	for _, name := range []string{"mini_ltp_agent.js", "agents/mini_ltp_agent.js"} {
		if _, err := ReadScript(name); err != nil {
			t.Fatalf("读取 %q 失败: %v", name, err)
		}
	}
}
