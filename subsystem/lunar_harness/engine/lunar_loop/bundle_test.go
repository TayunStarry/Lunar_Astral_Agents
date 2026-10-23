package lunarloop

import (
	"regexp"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/prompts"
)

// agentSystem.js 必须被内嵌且非空。
//
// 该脚本是月华智能体的思考循环本体（TypeScript/ 经 tsc + rollup 产出）。它由
// 直到运行期 RunAgentContext 才发现——而那时程序已在装配流程里，表现为
// "启动失败"，很难一眼看出只是构建产物没生成或名字不一致。
func TestAgentSystemBundleIsEmbedded(t *testing.T) {
	if len(agentSystemJS) == 0 {
		t.Fatal("内嵌的 agentSystem.js 为空（检查 engine/lunar_loop/variable.go 的 go:embed 与 npm run server.side 产物）")
	}
	// 兜底核对打包产物确实是智能体脚本而不是被替换成了别的东西
	for _, marker := range []string{"fileView", "prompts/"} {
		if !strings.Contains(string(agentSystemJS), marker) {
			t.Fatalf("内嵌的 agentSystem.js 缺少关键标识 %q", marker)
		}
	}
}

// 脚本里引用的**每一个**提示词文件都必须能被 prompts 包取到。
//
// 为什么值得单独钉住：绘制者/对话者等角色在**字段初始化**时就读提示词
// （`fileView('prompts/xxx.md')[0]`）。少一个文件不会编译失败，也不会在启动时报错，
// 而是在构造该角色的那一刻抛异常——表现为「某个工具/某个角色整体消失」，
// 排查要从 JS 堆栈一路倒回到内嵌资源，成本极高。
// 这条用例把「TS 里写了哪个文件名」与「实际内嵌了哪些文件」直接对齐。
func TestBundlePromptReferencesAreEmbedded(t *testing.T) {
	re := regexp.MustCompile(`prompts/([A-Za-z0-9_\-]+\.md)`)
	matches := re.FindAllStringSubmatch(string(agentSystemJS), -1)
	if len(matches) == 0 {
		t.Fatal("agentSystem.js 里没有引用任何 prompts/*.md：提示词很可能被改成硬编码，或打包产物异常")
	}
	seen := map[string]bool{}
	for _, m := range matches {
		name := m[1]
		if seen[name] {
			continue
		}
		seen[name] = true
		data, err := prompts.Read(name)
		if err != nil {
			t.Fatalf("脚本引用了 prompts/%s，但内嵌资源里取不到: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("脚本引用了 prompts/%s，但内嵌内容为空", name)
		}
	}
	// 提示词已模块化：公用预设 + 角色合并文档是立绘/合照的唯二依赖，
	// 漏掉任意一个说明打包时引用了不存在的文件
	for _, must := range []string{
		"painterCommon.md", "painterCharacters.md",
	} {
		if !seen[must] {
			t.Fatalf("脚本未引用 %s（绘制者提示词依赖它）", must)
		}
	}
}

// 打包产物的 IIFE 全局名不得叫 agentSystem。
//
// 为什么值得单独钉住：rollup 的 `--name agentSystem` 会生成顶层
// `var agentSystem = (function (exports) { … })({})`，这段赋值在脚本**末尾**执行，
// 会把 roles/index.ts 通过 `globalThis.agentSystem = Object.assign(…)` 挂上去的
// 角色实例与自愈入口（snapshotContext / restoreContext）整个覆盖掉。
// 后果不是崩溃而是**静默降级**：reload.go 每次「重载前抢救上下文」都拿到 undefined，
// 上下文白丢，只留一条容易被忽略的警告。静态钉住 globals 名字最省事。
func TestBundleIIFENameDoesNotShadowAgentSystem(t *testing.T) {
	bundle := string(agentSystemJS)
	head := strings.TrimLeft(bundle, " \t\r\n")
	const expected = "var lunarBundle = (function (exports)"
	if !strings.HasPrefix(head, expected) {
		preview := head
		if len(preview) > 80 {
			preview = preview[:80]
		}
		t.Fatalf("打包产物开头不是 %q；实际前 80 字符：%q\n"+
			"（npm run server.side 的 rollup --name 必须叫 lunarBundle，不能叫 agentSystem）", expected, preview)
	}
	if re := regexp.MustCompile(`(?m)^var agentSystem\b`); re.MatchString(bundle) {
		t.Fatal("打包产物里出现了顶层 `var agentSystem`：它会覆盖 roles/index.ts 挂到 globalThis.agentSystem 的自愈入口")
	}
	// 反向确认宿主全局挂载还在（roles/index.ts 的 Object.assign 不能被删掉）
	for _, marker := range []string{"snapshotContext", "restoreContext", "randomDefaultMessage"} {
		if !strings.Contains(bundle, marker) {
			t.Fatalf("打包产物里缺少 %q：roles/index.ts 的宿主全局挂载可能被删掉了", marker)
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
