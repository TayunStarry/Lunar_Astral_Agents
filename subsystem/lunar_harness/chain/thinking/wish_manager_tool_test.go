package thinking

// wish_manager_tool_test.go 愿望管理工具的注册、输入校验与提示词一致性验收。

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/internal/testenv"
	"LunarSubsystem/LunarHarness/prompts"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// decodeToolResult 解析工具统一输出原语。
func decodeToolResult(t *testing.T, result ToolResult) agentdispatch.Result {
	t.Helper()
	var decoded agentdispatch.Result
	if err := json.Unmarshal([]byte(result.Text), &decoded); err != nil {
		t.Fatalf("工具输出不是统一原语: %v（%q）", err, result.Text)
	}
	return decoded
}

// TestWishManagerRequiresInstruction 缺失 instruction 必须返回标准错误。
func TestWishManagerRequiresInstruction(t *testing.T) {
	decoded := decodeToolResult(t, Dialogue.ExecuteLifePlanning(map[string]any{
		"type": "write_wish", "handle": []any{}, "details": false,
	}))
	if !strings.Contains(decoded.Error, "instruction 不能为空") {
		t.Fatalf("缺失 instruction 应报错: %+v", decoded)
	}
	decoded = decodeToolResult(t, Dialogue.ExecuteLifePlanning(map[string]any{
		"type": "write_wish", "instruction": "   ", "handle": []any{}, "details": false,
	}))
	if !strings.Contains(decoded.Error, "instruction 不能为空") {
		t.Fatalf("空白 instruction 应报错: %+v", decoded)
	}
}

// TestWishManagerRejectsUnknownType 对话侧只提供 write_wish / wish_complete；
// 新增、改写、删除、调整优先级与查询都由 write_wish 自然语言承载。
func TestWishManagerRejectsUnknownType(t *testing.T) {
	for _, kind := range []string{"bad", "add", "update", "delete", "priority", "query"} {
		decoded := decodeToolResult(t, Dialogue.ExecuteLifePlanning(map[string]any{
			"type": kind, "instruction": "x", "handle": []any{}, "details": false,
		}))
		if !strings.Contains(decoded.Error, "type 只能取") {
			t.Fatalf("type %s 应被拒绝: %+v", kind, decoded)
		}
	}
}

// TestWishManagerDetails 聚合工具的 details 只读返回各机能描述。
func TestWishManagerDetails(t *testing.T) {
	cases := map[string]string{
		"write_wish":    "用自然语言描述你的愿望或什么时候想去做什么事",
		"wish_complete": "用自然语言描述你的什么愿望已达成",
	}
	for kind, want := range cases {
		decoded := decodeToolResult(t, Dialogue.ExecuteLifePlanning(map[string]any{
			"type": kind, "instruction": "x", "handle": []any{}, "details": true,
		}))
		if decoded.Error != "" || !strings.Contains(decoded.Result, want) {
			t.Fatalf("details 查询 %s 失败: %+v", kind, decoded)
		}
	}
}

// TestDialogueCatalogUsesLifePlanning 生活规划是唯一的生活入口。
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
	hasLifePlanning := false
	for _, name := range dialogueTools {
		switch name {
		case Dialogue.ToolLifePlanning:
			hasLifePlanning = true
		case "wish_manager", "life_planning_legacy", "schedule":
			t.Fatalf("对话者工具目录仍残留旧入口 %s: %v", name, dialogueTools)
		}
	}
	if !hasLifePlanning {
		t.Fatalf("对话者工具目录缺少 life_planning: %v", dialogueTools)
	}
}

// TestDialoguePromptUsesLifePlanning 提示词只保留生活规划，不再提旧愿望入口。
func TestDialoguePromptUsesLifePlanning(t *testing.T) {
	raw, err := prompts.Read("dialogueRole.md")
	if err != nil {
		t.Fatalf("读取对话者提示词失败: %v", err)
	}
	prompt := string(raw)
	if !strings.Contains(prompt, "`life_planning`") {
		t.Fatalf("提示词未使用 life_planning: %s", prompt)
	}
	for _, description := range []string{
		"用自然语言描述你的愿望或什么时候想去做什么事",
		"用自然语言描述你的什么愿望已达成",
	} {
		if !strings.Contains(prompt, description) {
			t.Fatalf("提示词缺少愿望机能描述 %q", description)
		}
	}
	for _, banned := range []string{"`wish_manager`", "recent_activity", "record_intent", "`type`=`query`"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("提示词仍残留旧入口 %s", banned)
		}
	}
	for _, banned := range []string{"`type`=`add`", "`type`=`update`", "`type`=`delete`", "`type`=`priority`"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("提示词仍残留细分愿望类型 %s", banned)
		}
	}
}

// TestRuntimeMessageCarriesWishList 运行时消息必须始终展示完整愿望清单。
func TestRuntimeMessageCarriesWishList(t *testing.T) {
	message := buildRuntimeGlobalMessage()
	text, _ := message.Content.(string)
	if !strings.Contains(text, "【愿望清单】") {
		t.Fatalf("运行时消息缺少愿望清单：%s", text)
	}
	for _, banned := range []string{"未来一小时计划", "计划表"} {
		if strings.Contains(text, banned) {
			t.Fatalf("运行时消息仍注入旧计划语义 %q：%s", banned, text)
		}
	}
}

// TestRuntimeMessageCarriesObsession 逾期愿望在运行时消息里整合为唯一执念。
func TestRuntimeMessageCarriesObsession(t *testing.T) {
	testenv.Isolate(t)
	wishchain.Reset()
	t.Cleanup(wishchain.Reset)
	past := time.Now().Add(-wishchain.GraceDuration - time.Hour).Format("2006-01-02 15:04:05")
	if _, err := wishchain.Add("看《流浪地球》", past, wishchain.AutoPriority); err != nil {
		t.Fatal(err)
	}
	message := buildRuntimeGlobalMessage()
	text, _ := message.Content.(string)
	if !strings.Contains(text, "已逾期") {
		t.Fatalf("运行时愿望清单应标记已逾期：%s", text)
	}
	if !strings.Contains(text, "【执念】请寻找办法完成已失约的") || !strings.Contains(text, "看《流浪地球》") {
		t.Fatalf("运行时消息缺少执念提示：%s", text)
	}
}

// TestFileOperationsBranchMigratedToLifePlanning 文件操作已迁移到生活规划入口。
func TestFileOperationsBranchMigratedToLifePlanning(t *testing.T) {
	decoded := decodeToolResult(t, Dialogue.ExecuteLifePlanning(map[string]any{
		"type": "file_operations", "details": true, "handle": []any{},
	}))
	if decoded.Error != "" || !strings.Contains(decoded.Result, "文件操作") {
		t.Fatalf("file_operations 职能描述不正确: %+v", decoded)
	}
	// 旧的技法聚合入口不再暴露 storyboard / file_operations。
	legacy := decodeToolResult(t, Dialogue.ExecuteDispatchTechnique(map[string]any{
		"type": "file_operations", "details": true, "handle": []any{},
	}))
	if legacy.Error == "" {
		t.Fatalf("dispatch_technique 不应再接受 file_operations: %+v", legacy)
	}
	raw, err := prompts.Read("dialogueRole.md")
	if err != nil {
		t.Fatalf("读取对话者提示词失败: %v", err)
	}
	prompt := string(raw)
	if !strings.Contains(prompt, "`file_operations`") {
		t.Fatalf("对话者提示词未使用 file_operations: %s", prompt)
	}
	if strings.Contains(prompt, "`type`=`creator`") {
		t.Fatal("对话者提示词仍残留 creator 分支名")
	}
}
