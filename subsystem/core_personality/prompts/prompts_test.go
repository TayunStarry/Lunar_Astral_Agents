package prompts

import "testing"

// 全部提示词必须都能从 prompts 包的内嵌资源里取到，且内容非空。
//
// 月华的角色提示词就是人格设定本体：Read 取不到时会明确报错，但若返回空串，
// 模型会静默失去该角色的人格设定（对话照常、行为跑偏），比报错难排查得多。
func TestReadServesAllPrompts(t *testing.T) {
	names := []string{
		"appearanceBase.md",
		"crystalAppearance.md",
		"descriptionRole.md",
		"dialogueRole.md",
		"memorizerRole.md",
		"memorizerSummaryTask.md",
		"musicianRole.md",
		"painterRole.md",
		"perceiverRole.md",
		"perceiverSummaryTask.md",
		"selfAppearanceChibi.md",
		"selfAppearanceFull.md",
		"selfAppearanceGroup.md",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			data, err := Read(name)
			if err != nil {
				t.Fatalf("读取内嵌提示词 %s 失败: %v", name, err)
			}
			if len(data) == 0 {
				t.Fatalf("内嵌提示词 %s 为空", name)
			}
		})
	}
}

// TypeScript 侧以 `fileView('prompts/xxx.md')` 的虚拟路径引用提示词，
// 因此带 "prompts/" 前缀的写法必须与不带前缀等价。
//
// 非提示词前缀必须报错：提示词已从"前端静态资源"目录迁出，若仍按
// web/、assets/ 去取，说明虚拟路径契约与内嵌布局又错开了。
func TestReadPathHandling(t *testing.T) {
	if _, err := Read(""); err == nil {
		t.Fatal("空提示词名称应当报错")
	}

	for _, bad := range []string{
		"web/dialogueRole.md",            // 历史位置（琉璃桌面前端资源）
		"assets/prompts/dialogueRole.md", // 历史位置（月华 hierarchy 资源树）
		"index.html",                     // 非提示词
		"dialogueRole",                   // 缺扩展名
		"不存在的提示词.md",
	} {
		if data, err := Read(bad); err == nil {
			t.Fatalf("%q 不应被解析为提示词（取到 %d 字节）", bad, len(data))
		}
	}

	for _, name := range []string{"dialogueRole.md", "prompts/dialogueRole.md"} {
		if _, err := Read(name); err != nil {
			t.Fatalf("读取 %q 失败: %v", name, err)
		}
	}
}
