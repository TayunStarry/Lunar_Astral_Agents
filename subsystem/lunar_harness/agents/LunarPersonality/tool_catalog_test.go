package LunarPersonality

// tool_catalog_test.go 钉住工具目录的两个事实：
//  1. 记忆者不是独立智能体，不出现在工具列表；
//  2. 对话者的 LTPX 聚合工具 use_the_program 必须出现在清单里。

import "testing"

func TestToolCatalogIncludesLTPXAndExcludesMemorizer(t *testing.T) {
	catalog := ToolCatalog()

	for _, agent := range catalog {
		if agent.ID == "memorizer" || agent.Name == "记忆者" {
			t.Fatalf("记忆者只是记忆入库流程，不应作为智能体出现在工具列表: %#v", agent)
		}
	}

	var dialogueTools []string
	for _, agent := range catalog {
		if agent.ID != "dialogue" {
			continue
		}
		for _, tool := range agent.Tools {
			dialogueTools = append(dialogueTools, tool.Name)
		}
	}
	if len(dialogueTools) == 0 {
		t.Fatal("对话者工具清单为空")
	}
	found := false
	for _, name := range dialogueTools {
		if name == crystalToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("对话者工具清单缺少 LTPX 聚合工具 %s：%v", crystalToolName, dialogueTools)
	}
}
