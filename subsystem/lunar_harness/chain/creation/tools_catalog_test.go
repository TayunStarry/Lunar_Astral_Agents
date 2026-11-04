package creation

// tools_catalog_test.go 创作链工具定义派生自智能体注册表的覆盖性自检。

import "testing"

func TestDefinitionsCoverToolSets(t *testing.T) {
	sets := map[string][]string{
		"planner":    plannerTools,
		"writer":     writerTools,
		"engineer":   engineerTools,
		"researcher": researcherTools,
		"runner":     runnerTools,
	}
	for label, names := range sets {
		defs := definitionsFor(names)
		if len(defs) != len(names) {
			t.Fatalf("%s 工具集定义数 = %d，期望 %d", label, len(defs), len(names))
		}
		for i, def := range defs {
			if def.Function.Name != names[i] {
				t.Fatalf("%s 第 %d 个工具 = %s，期望 %s（顺序必须与清单一致）",
					label, i, def.Function.Name, names[i])
			}
		}
	}
}
