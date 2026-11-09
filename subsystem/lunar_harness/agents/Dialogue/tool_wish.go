package Dialogue

// tool_wish.go 愿望管理聚合工具执行器：按 type 把自然语言愿望指令交给愿望管理智能体。
//
// 输入输出严格遵循聚合工具的统一样式：type 选机能，instruction 写自然语言，
// handle 不使用（传空数组），details 只读返回职能描述。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/WishManager"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// wishManagerTimeout 单次愿望管理任务超时
	wishManagerTimeout = 5 * time.Minute

	// wishTypeAdd 新增愿望
	wishTypeAdd = "add"
	// wishTypeUpdate 改写愿望
	wishTypeUpdate = "update"
	// wishTypeComplete 标记愿望完成
	wishTypeComplete = "complete"
	// wishTypeDelete 删除愿望
	wishTypeDelete = "delete"
	// wishTypePriority 调整愿望优先级
	wishTypePriority = "priority"
)

// wishTypeOrder 聚合工具 type 的固定顺序。
//
// 查询不在对话侧提供：运行时消息每轮已经展示完整愿望清单与 ID，
// 对话者问「有什么愿望」时直接依据运行时消息回答即可。
var wishTypeOrder = []string{
	wishTypeAdd, wishTypeUpdate, wishTypeComplete,
	wishTypeDelete, wishTypePriority,
}

// wishTypeScopes 各机能的职能描述（details=true 专用）。
var wishTypeScopes = map[string]string{
	wishTypeAdd:      "新增愿望：instruction 写清愿望内容、期望达成时间与优先级（优先级可省略，0 最高）。",
	wishTypeUpdate:   "改写愿望：instruction 写清要改哪一条、改成什么内容或时间。",
	wishTypeComplete: "完成愿望：instruction 写清要标记完成的愿望。",
	wishTypeDelete:   "删除愿望：instruction 写清要删除的愿望。",
	wishTypePriority: "调整优先级：instruction 写清愿望与目标优先级（0-15，0 最高）。",
}

// wishTypeDirectives 聚合 type → 交给愿望管理智能体的操作提示。
var wishTypeDirectives = map[string]string{
	wishTypeAdd:      "【本轮操作类型】新增愿望：请优先使用 wish_add。",
	wishTypeUpdate:   "【本轮操作类型】改写愿望：请优先使用 wish_update。",
	wishTypeComplete: "【本轮操作类型】标记愿望完成：请优先使用 wish_complete。",
	wishTypeDelete:   "【本轮操作类型】删除愿望：请优先使用 wish_delete。",
	wishTypePriority: "【本轮操作类型】调整愿望优先级：请优先使用 wish_priority。",
}

// ExecuteWishManager 执行一次愿望管理聚合工具调用。
func ExecuteWishManager(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = toolJSON(agentdispatch.Failure(fmt.Sprintf("愿望管理执行异常: %v", recovered), nil))
		}
	}()
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	scope, ok := wishTypeScopes[kind]
	if !ok {
		return toolJSON(agentdispatch.Failure("type 只能取 "+strings.Join(wishTypeOrder, " / "), nil))
	}
	if call.Details {
		return toolJSON(agentdispatch.Success(scope, nil))
	}
	if call.Instruction == "" {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要对愿望做什么", nil))
	}

	ctx, cancel := context.WithTimeout(context.Background(), wishManagerTimeout)
	defer cancel()
	outcome, err := WishManager.Execute(ctx, wishTypeDirectives[kind]+"\n"+call.Instruction)
	if err != nil {
		return toolJSON(agentdispatch.PartialFailure("愿望管理失败："+err.Error(), strings.TrimSpace(outcome.Summary), outcome.Handles))
	}
	// 容量满时不替对话者做取舍：把完整列表回填给模型，由它向对话者确认。
	if outcome.NeedConfirm {
		summary := strings.TrimSpace(outcome.Summary)
		if list, ok := outcome.Detail["wishes"].([]string); ok && len(list) > 0 {
			summary += "\n当前愿望：\n" + strings.Join(list, "\n")
		}
		return toolJSON(agentdispatch.Success(summary, outcome.Handles))
	}
	if !outcome.OK {
		summary := strings.TrimSpace(outcome.Summary)
		if summary == "" {
			summary = "愿望管理未成功"
		}
		return toolJSON(agentdispatch.PartialFailure(summary, "", outcome.Handles))
	}
	summary := strings.TrimSpace(outcome.Summary)
	if summary == "" {
		summary = "愿望管理已完成"
	}
	return toolJSON(agentdispatch.Success(summary, outcome.Handles))
}
