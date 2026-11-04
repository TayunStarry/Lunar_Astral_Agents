package thinking

// life_planning_tool_test.go life_planning 三个子能力的收发与改名后的调用链验收。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/chain/life"
	"LunarSubsystem/LunarHarness/internal/testenv"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// decodeLifePlanning 解析统一输出原语。
func decodeLifePlanning(t *testing.T, result ToolResult) agentdispatch.Result {
	t.Helper()
	var decoded agentdispatch.Result
	if err := json.Unmarshal([]byte(result.Text), &decoded); err != nil {
		t.Fatalf("生活规划输出不是统一原语: %v（%q）", err, result.Text)
	}
	return decoded
}

// TestLifePlanningDetails 三个子能力都能只读返回职能描述。
func TestLifePlanningDetails(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{lifePlanningPlanManager, "计划管理"},
		{lifePlanningRecentActivity, "最近活动"},
		{lifePlanningRecordIntent, "创作意图"},
	}
	for _, tc := range cases {
		decoded := decodeLifePlanning(t, handleLifePlanning(map[string]any{
			"type": tc.kind, "details": true, "handle": []any{},
		}))
		if decoded.Error != "" || !strings.Contains(decoded.Result, tc.want) {
			t.Fatalf("details 查询 %s 失败: %+v", tc.kind, decoded)
		}
	}
}

// TestLifePlanningRecordIntent 记录意图会覆盖生活创作参考字符串。
func TestLifePlanningRecordIntent(t *testing.T) {
	testenv.Isolate(t)
	decoded := decodeLifePlanning(t, handleLifePlanning(map[string]any{
		"type": lifePlanningRecordIntent, "instruction": "下一轮写一首关于秋天的诗", "handle": []any{},
	}))
	if decoded.Error != "" || !strings.Contains(decoded.Result, "秋天的诗") {
		t.Fatalf("记录创作意图失败: %+v", decoded)
	}
	if got := life.CreativeIntent(); got != "下一轮写一首关于秋天的诗" {
		t.Fatalf("参考字符串未写入: %q", got)
	}
	_ = life.ResetCreativeIntent()
}

// TestLifePlanningRecentActivity 最近活动按需读取，空状态有明确返回。
func TestLifePlanningRecentActivity(t *testing.T) {
	testenv.Isolate(t)
	decoded := decodeLifePlanning(t, handleLifePlanning(map[string]any{
		"type": lifePlanningRecentActivity, "handle": []any{},
	}))
	if decoded.Error != "" || decoded.Result == "" {
		t.Fatalf("最近活动应有明确返回: %+v", decoded)
	}
}

// TestLifePlanningRejectsBadInput 未知分支与缺失 instruction 必须返回标准错误。
func TestLifePlanningRejectsBadInput(t *testing.T) {
	badType := decodeLifePlanning(t, handleLifePlanning(map[string]any{
		"type": "bad", "instruction": "x", "handle": []any{},
	}))
	if !strings.Contains(badType.Error, "type 只能取") {
		t.Fatalf("未知分支应报错: %+v", badType)
	}
	missing := decodeLifePlanning(t, handleLifePlanning(map[string]any{
		"type": lifePlanningPlanManager, "handle": []any{},
	}))
	if !strings.Contains(missing.Error, "instruction 不能为空") {
		t.Fatalf("缺失 instruction 应报错: %+v", missing)
	}
}

// TestFileOperationsBranchAndLegacyAlias 改名后的分支可用，旧值仍兼容。
func TestFileOperationsBranchAndLegacyAlias(t *testing.T) {
	for _, kind := range []string{techniqueFileOperations, techniqueCreatorLegacy, techniqueFileEditorLegacy} {
		decoded := decodeLifePlanning(t, ToolResult{Text: handleDispatchTechnique(map[string]any{
			"type": kind, "details": true, "handle": []any{},
		}).Text})
		if decoded.Error != "" || !strings.Contains(decoded.Result, "文件操作") {
			t.Fatalf("分支 %s 的职能描述不正确: %+v", kind, decoded)
		}
	}
	prompt, err := prompts.Read("dialogueRole.md")
	if err != nil {
		t.Fatalf("读取对话者提示词失败: %v", err)
	}
	if !strings.Contains(string(prompt), "`type`=`file_operations`") {
		t.Fatalf("对话者提示词未使用 file_operations: %s", string(prompt))
	}
	if strings.Contains(string(prompt), "`type`=`creator`") {
		t.Fatal("对话者提示词仍残留 creator 分支名")
	}
}

// TestDialogueCatalogUsesLifePlanning 旧 schedule 工具下线，计划表统一走 life_planning。
func TestDialogueCatalogUsesLifePlanning(t *testing.T) {
	catalog := ToolCatalog()
	var dialogueTools []string
	for _, agent := range catalog {
		if agent.ID != "dialogue" {
			continue
		}
		for _, tool := range agent.Tools {
			dialogueTools = append(dialogueTools, tool.Name)
		}
	}
	hasPlanning, hasOldSchedule := false, false
	for _, name := range dialogueTools {
		switch name {
		case lifePlanningToolName:
			hasPlanning = true
		case "schedule":
			hasOldSchedule = true
		}
	}
	if !hasPlanning {
		t.Fatalf("对话者工具目录缺少 life_planning: %v", dialogueTools)
	}
	if hasOldSchedule {
		t.Fatalf("对话者工具目录仍残留 schedule: %v", dialogueTools)
	}
}

// TestRuntimeMessageOmitsLifeAndPlan 最近生活与计划摘要不再注入运行时消息。
func TestRuntimeMessageOmitsLifeAndPlan(t *testing.T) {
	message := buildRuntimeGlobalMessage()
	text, _ := message.Content.(string)
	for _, banned := range []string{"近期生活", "未来一小时计划", "最近做过"} {
		if strings.Contains(text, banned) {
			t.Fatalf("运行时消息仍注入 %q：%s", banned, text)
		}
	}
}
