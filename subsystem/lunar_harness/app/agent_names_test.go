package app

// agent_names_test.go 校验显示名唯一来源：
// 普通智能体统一为四个汉字；「分镜规划-*」按「家族-阶段」格式登记；
// 工具目录不得出现表外名字。

import (
	"strings"
	"testing"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
	"LunarSubsystem/LunarHarness/protocol/toolreg"
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
		// 家族式角色名使用「家族-阶段」格式（如 分镜规划-拆解）。
		if prefix, stage, ok := strings.Cut(name, "-"); ok {
			if prefix == "" || stage == "" {
				t.Fatalf("智能体 %s 的家族名格式不正确：%q", id, name)
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

// roleAgentIDsAllowNonAgent roleAgentIDs 中指向链 / 执行体的修复目标。
//
// agentinfo.DisplayNames 只登记 Go 智能体，以下三个目标由各自的目录与导出注册
// 提供显示名，因此不参与「修复目标必须在 DisplayNames 登记」的断言：
//   - archive_reader：归档解析执行体（executors/ArchiveReader）
//   - bilibili：哔哩观影链（chain/viewer 的导出目标）
//   - web_search：网络搜索链（chain/search）
var roleAgentIDsAllowNonAgent = map[string]struct{}{
	"archive_reader": {},
	"bilibili":       {},
	"web_search":     {},
}

// TestRoleAgentIDsRegistered 校验故障修复映射的每个目标都在显示名表登记。
// 漏登记时修复会落到没有显示名的目标上，与实况上报的 ID 对不上。
func TestRoleAgentIDsRegistered(t *testing.T) {
	for role, id := range roleAgentIDs {
		if _, ok := agentinfo.DisplayNames[id]; ok {
			continue
		}
		if _, ok := roleAgentIDsAllowNonAgent[id]; ok {
			continue
		}
		t.Errorf("roleAgentIDs[%q] = %q 未在 protocol/agentinfo/names.go 的 DisplayNames 登记：故障修复会落到无显示名的目标", role, id)
	}
}

// TestToolOwnersPresentInCatalog 校验在 toolreg 登记了工具的智能体都进了工具目录。
// 漏登记时控制台「工具列表」屏看不到该智能体。
func TestToolOwnersPresentInCatalog(t *testing.T) {
	catalog := buildToolCatalog()
	inCatalog := make(map[string]struct{}, len(catalog))
	for _, agent := range catalog {
		inCatalog[agent.ID] = struct{}{}
	}
	for _, owner := range toolreg.Owners() {
		if _, ok := inCatalog[owner]; ok {
			continue
		}
		t.Errorf("智能体 %q 在 protocol/toolreg 登记了工具，但 buildToolCatalog() 的结果里没有它：控制台工具列表看不到", owner)
	}
}
