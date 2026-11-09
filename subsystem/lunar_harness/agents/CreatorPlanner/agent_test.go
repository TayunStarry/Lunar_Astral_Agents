package CreatorPlanner

// agent_test.go 创作规划者的工具声明与提示词自检。

import (
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/protocol/toolreg"
)

func TestToolSchemasRegistered(t *testing.T) {
	schemas := toolreg.Schemas(AgentID)
	if len(schemas) != 21 {
		t.Fatalf("登记的工具声明数 = %d，期望 21", len(schemas))
	}
	for _, name := range []string{"submit_plan", "file_read", "fetch_resources", "run_script", "send_private_text"} {
		if _, ok := toolreg.LookupSchema(name); !ok {
			t.Fatalf("工具 %s 未在 toolreg 登记", name)
		}
	}
}

func TestPlannerPromptComposesModeAndHistory(t *testing.T) {
	prompt := PlannerPrompt(false, []string{"写一首短诗"}, 2, 1, 1, 3, "")
	for _, want := range []string{
		"你是「创作调度」",
		"【本轮创作参考（来自对话者给出的期望",
		"- 写一首短诗",
		"【本次为自由创作模式】",
		"历史概况：上一轮 2 步规划、1 条备忘、1 条错误、3 条内容级变更。",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("调度者提示缺少 %q", want)
		}
	}

	assigned := PlannerPrompt(true, nil, 0, 0, 0, 0, "")
	if !strings.Contains(assigned, "【本次为对话者指派的任务】") {
		t.Fatalf("指派模式提示缺少模式说明")
	}
	if !strings.Contains(assigned, "这是第一次指派任务") {
		t.Fatalf("指派模式提示缺少首次历史说明")
	}
}

func TestPlannerPromptInjectsPathSnapshot(t *testing.T) {
	snapshot := "documents：1 个文件 / 1 个目录\nunknown：0 个文件 / 0 个目录\n--- 相对路径 ---\ndocuments/\n  a.md"
	prompt := PlannerPrompt(false, nil, 0, 0, 0, 0, snapshot)
	if !strings.Contains(prompt, "【已有文件路径快照") {
		t.Fatalf("调度者提示缺少路径快照标题")
	}
	if !strings.Contains(prompt, snapshot) {
		t.Fatalf("调度者提示缺少路径快照正文")
	}
	if empty := PlannerPrompt(false, nil, 0, 0, 0, 0, "  "); strings.Contains(empty, "【已有文件路径快照") {
		t.Fatalf("快照为空时不应注入标题")
	}
}

func TestExecutorPromptKeepsRoleBlocks(t *testing.T) {
	for agent, want := range map[string]string{
		AgentWriter:     "你是文字创作者",
		AgentEngineer:   "你是工程创作者",
		AgentResearcher: "你是资料查阅者",
		AgentRunner:     "你是脚本运行者",
	} {
		prompt := ExecutorPrompt(agent)
		if !strings.Contains(prompt, "月华.生活.自由创作") || !strings.Contains(prompt, want) {
			t.Fatalf("%s 提示不完整", agent)
		}
	}
	if prompt := ExecutorPrompt("unknown"); !strings.Contains(prompt, "月华.生活.自由创作") {
		t.Fatalf("未知角色应回退到通用提示")
	}
}
