package lunarloop

import (
	"strings"
	"testing"
)

// agentSystem.js 必须被内嵌且非空。
//
// 该脚本是月华智能体的思考循环本体（TypeScript/ 经 tsc + rollup 产出）。它由
// go:embed 编进二进制，一旦 embed 模式与实际产物名不符，编译期毫无提示，
// 直到运行期 RunAgentContext 才发现——而那时程序已在装配流程里，表现为
// "启动失败"，很难一眼看出只是构建产物没生成或名字不一致。
func TestAgentSystemBundleIsEmbedded(t *testing.T) {
	if len(agentSystemJS) == 0 {
		t.Fatal("内嵌的 agentSystem.js 为空（检查 engine/LunarLoop/variable.go 的 go:embed 与 npm run server.side 产物）")
	}
	// 兜底核对打包产物确实是智能体脚本而不是被替换成了别的东西
	for _, marker := range []string{"fileView", "prompts/"} {
		if !strings.Contains(string(agentSystemJS), marker) {
			t.Fatalf("内嵌的 agentSystem.js 缺少关键标识 %q", marker)
		}
	}
}

// fileView 的虚拟路径分派必须只接受已知前缀，且已知前缀必须能真正取到内容。
//
// 这条守的是 TypeScript ←→ Go 的隐式契约：TS 侧按 `prompts/xxx.md` 取值，
// Go 侧按前缀分派到 prompts 包。契约两端分居两个语言、两种目录，改任一端都
// 不会有编译期告警，只能靠测试钉住。
func TestReadEmbeddedAssetRouting(t *testing.T) {
	data, err := readEmbeddedAsset("prompts/dialogueRole.md")
	if err != nil {
		t.Fatalf("按 prompts/ 前缀读取提示词失败: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("按 prompts/ 前缀读到的提示词为空")
	}

	for _, bad := range []string{
		"",                               // 空路径
		"assets/prompts/dialogueRole.md", // 历史前缀
		"web/index.html",                 // 琉璃前端资源不属于引擎可读范围
		"agentSystem.js",                 // 脚本走 go:embed，不经 fileView
		"dialogueRole.md",                // 必须带前缀
	} {
		if got, err := readEmbeddedAsset(bad); err == nil {
			t.Fatalf("%q 不应被解析为内嵌资源（取到 %d 字节）", bad, len(got))
		}
	}
}
