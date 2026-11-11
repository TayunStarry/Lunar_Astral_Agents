package prompts

import (
	"strings"
	"testing"
)

// prompts_test.go 验证基础人设只有一份：各链通过 {base-persona} 引用
// dialogueRole.md 的「我是谁」段落，且不携带对话场景独有的占位符。

func TestBasePersonaInjectedIntoChains(t *testing.T) {
	consumers := []string{
		"lifeRole.md",
		"storyboardPlanner.md",
	}
	for _, name := range consumers {
		data, err := Read(name)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", name, err)
		}
		text := string(data)
		if strings.Contains(text, BasePersonaPlaceholder) {
			t.Fatalf("%s 仍残留 %s 占位符", name, BasePersonaPlaceholder)
		}
		if !strings.Contains(text, "俏皮自信") {
			t.Fatalf("%s 未注入基础人设", name)
		}
		if strings.Contains(text, "{current-address}") {
			t.Fatalf("%s 不应携带对话场景的当前位置占位符", name)
		}
	}
}

func TestBasePersonaSingleSource(t *testing.T) {
	persona := BasePersona()
	for _, fragment := range []string{"名字：月华", "俏皮自信", "家人"} {
		if !strings.Contains(persona, fragment) {
			t.Fatalf("基础人设缺少 %q: %s", fragment, persona)
		}
	}
	if strings.Contains(persona, "当前位置") {
		t.Fatalf("基础人设不应包含对话场景的当前位置: %s", persona)
	}
}
