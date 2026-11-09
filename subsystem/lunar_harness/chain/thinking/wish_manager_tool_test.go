package thinking

// wish_manager_tool_test.go 愿望管理工具的注册、输入校验与提示词一致性验收。

import (
	"encoding/json"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/agents/Dialogue"
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
	decoded := decodeToolResult(t, Dialogue.ExecuteWishManager(map[string]any{
		"type": "add", "handle": []any{}, "details": false,
	}))
	if !strings.Contains(decoded.Error, "instruction 不能为空") {
		t.Fatalf("缺失 instruction 应报错: %+v", decoded)
	}
	decoded = decodeToolResult(t, Dialogue.ExecuteWishManager(map[string]any{
		"type": "add", "instruction": "   ", "handle": []any{}, "details": false,
	}))
	if !strings.Contains(decoded.Error, "instruction 不能为空") {
		t.Fatalf("空白 instruction 应报错: %+v", decoded)
	}
}

// TestWishManagerRejectsUnknownType 未知 type 必须返回标准错误；
// query 不在对话侧提供，愿望清单直接来自运行时消息。
func TestWishManagerRejectsUnknownType(t *testing.T) {
	for _, kind := range []string{"bad", "query"} {
		decoded := decodeToolResult(t, Dialogue.ExecuteWishManager(map[string]any{
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
		"add":      "新增愿望",
		"update":   "改写愿望",
		"complete": "完成愿望",
		"delete":   "删除愿望",
		"priority": "调整优先级",
	}
	for kind, want := range cases {
		decoded := decodeToolResult(t, Dialogue.ExecuteWishManager(map[string]any{
			"type": kind, "instruction": "x", "handle": []any{}, "details": true,
		}))
		if decoded.Error != "" || !strings.Contains(decoded.Result, want) {
			t.Fatalf("details 查询 %s 失败: %+v", kind, decoded)
		}
	}
}

// TestDialogueCatalogUsesWishManager 愿望管理是唯一的生活规划入口。
func TestDialogueCatalogUsesWishManager(t *testing.T) {
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
	hasWishManager := false
	for _, name := range dialogueTools {
		switch name {
		case Dialogue.ToolWishManager:
			hasWishManager = true
		case "life_planning", "schedule":
			t.Fatalf("对话者工具目录仍残留旧入口 %s: %v", name, dialogueTools)
		}
	}
	if !hasWishManager {
		t.Fatalf("对话者工具目录缺少 wish_manager: %v", dialogueTools)
	}
}

// TestDialoguePromptUsesWishManager 提示词只保留愿望管理，不再提最近活动与创作意图。
func TestDialoguePromptUsesWishManager(t *testing.T) {
	raw, err := prompts.Read("dialogueRole.md")
	if err != nil {
		t.Fatalf("读取对话者提示词失败: %v", err)
	}
	prompt := string(raw)
	if !strings.Contains(prompt, "`wish_manager`") {
		t.Fatalf("提示词未使用 wish_manager: %s", prompt)
	}
	for _, kind := range []string{"add", "update", "complete", "delete", "priority"} {
		if !strings.Contains(prompt, "`type`=`"+kind+"`") {
			t.Fatalf("提示词缺少愿望机能 %s", kind)
		}
	}
	if strings.Contains(prompt, "`type`=`query`") {
		t.Fatal("提示词不应再提供对话侧查询入口")
	}
	for _, banned := range []string{"life_planning", "recent_activity", "record_intent"} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("提示词仍残留旧入口 %s", banned)
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

// TestFileOperationsBranchAndLegacyAlias 文件操作分支可用，旧值仍兼容。
func TestFileOperationsBranchAndLegacyAlias(t *testing.T) {
	for _, kind := range []string{Dialogue.TechniqueFileOperations, Dialogue.TechniqueCreatorLegacy, Dialogue.TechniqueFileEditorLegacy} {
		decoded := decodeToolResult(t, Dialogue.ExecuteDispatchTechnique(map[string]any{
			"type": kind, "details": true, "handle": []any{},
		}))
		if decoded.Error != "" || !strings.Contains(decoded.Result, "文件操作") {
			t.Fatalf("分支 %s 的职能描述不正确: %+v", kind, decoded)
		}
	}
	raw, err := prompts.Read("dialogueRole.md")
	if err != nil {
		t.Fatalf("读取对话者提示词失败: %v", err)
	}
	prompt := string(raw)
	if !strings.Contains(prompt, "`type`=`file_operations`") {
		t.Fatalf("对话者提示词未使用 file_operations: %s", prompt)
	}
	if strings.Contains(prompt, "`type`=`creator`") {
		t.Fatal("对话者提示词仍残留 creator 分支名")
	}
}
