package app

// agent_names_test.go 校验显示名唯一来源：
// 普通智能体统一为四个汉字；「分镜规划-*」按「家族-阶段」格式登记；
// 工具目录不得出现表外名字。

import (
	"strings"
	"testing"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

// storyboardFamilyPrefix 演绎链分镜规划家族的角色名前缀。
const storyboardFamilyPrefix = "分镜规划-"

func TestAgentDisplayNames(t *testing.T) {
	seen := make(map[string]string, len(agentinfo.DisplayNames))
	for id, name := range agentinfo.DisplayNames {
		if id == "" || name == "" {
			t.Fatalf("显示名登记不完整：id=%q name=%q", id, name)
		}
		if prev, ok := seen[name]; ok {
			t.Fatalf("显示名重复：%q 同时属于 %s 与 %s", name, prev, id)
		}
		seen[name] = id

		if strings.HasPrefix(name, storyboardFamilyPrefix) {
			if stage := strings.TrimPrefix(name, storyboardFamilyPrefix); stage == "" {
				t.Fatalf("智能体 %s 的家族名缺少阶段：%q", id, name)
			}
			continue
		}
		// 「记忆整理」是用户点名的固定名，按汉字计为 4 个；其余同样统一为 4 个汉字。
		if got := utf8.RuneCountInString(name); got != 4 {
			t.Fatalf("智能体 %s 的显示名应为 4 个汉字，实际 %d：%q", id, got, name)
		}
	}
}

func TestLifeMemorizerFixedName(t *testing.T) {
	if got := agentinfo.Name("life_memorizer"); got != "记忆整理" {
		t.Fatalf("记忆整理必须保持用户点名的固定名，实际 %q", got)
	}
}

func TestToolCatalogNamesFromRegistry(t *testing.T) {
	known := make(map[string]struct{}, len(agentinfo.DisplayNames))
	for _, name := range agentinfo.DisplayNames {
		known[name] = struct{}{}
	}

	for _, agent := range buildToolCatalog() {
		if agent.Kind == agentinfo.KindChain || agent.Kind == agentinfo.KindExecutor {
			continue
		}
		if _, ok := known[agent.Name]; !ok {
			t.Fatalf("智能体 %s 的目录名 %q 不在 agentinfo.DisplayNames 中", agent.ID, agent.Name)
		}
	}
}
