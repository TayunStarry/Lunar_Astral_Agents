package Dialogue

// tool_life.go 生活规划聚合工具执行器：计划表管理、最近活动查看、创作意图记录。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/PlanManager"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// LifePlanningPlanManager 计划表增删改查
	LifePlanningPlanManager = "plan_manager"
	// LifePlanningRecentActivity 查看最近做了什么
	LifePlanningRecentActivity = "recent_activity"
	// LifePlanningRecordIntent 记录空闲时的创作意图
	LifePlanningRecordIntent = "record_intent"
	// lifePlanningTimeout 单次计划管理任务超时
	lifePlanningTimeout = 5 * time.Minute
)

// ExecuteLifePlanning 执行生活规划聚合工具。
func ExecuteLifePlanning(args map[string]any) (result ToolResult) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = toolJSON(agentdispatch.Failure(fmt.Sprintf("生活规划执行异常: %v", recovered), nil))
		}
	}()
	call, err := agentdispatch.Parse(args)
	if err != nil {
		return toolJSON(agentdispatch.Failure(err.Error(), nil))
	}
	kind := strings.ToLower(call.Type)
	if call.Details {
		if text, ok := lifePlanningScope(kind); ok {
			return toolJSON(agentdispatch.Success(text, nil))
		}
		return toolJSON(agentdispatch.Failure("type 只能取 plan_manager / recent_activity / record_intent", nil))
	}
	if call.Instruction == "" && kind != LifePlanningRecentActivity {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要做什么", nil))
	}

	switch kind {
	case LifePlanningPlanManager:
		return toolJSON(dispatchPlanManager(call.Instruction))
	case LifePlanningRecentActivity:
		return toolJSON(recentActivityResult())
	case LifePlanningRecordIntent:
		return toolJSON(recordCreativeIntent(call.Instruction))
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 plan_manager / recent_activity / record_intent", nil))
	}
}

// dispatchPlanManager 把计划表任务交给独立计划管理智能体。
func dispatchPlanManager(instruction string) agentdispatch.Result {
	ctx, cancel := context.WithTimeout(context.Background(), lifePlanningTimeout)
	defer cancel()
	result, err := PlanManager.Execute(ctx, instruction)
	if err != nil {
		return agentdispatch.PartialFailure("计划管理失败："+err.Error(), strings.TrimSpace(result.Summary), result.Handles)
	}
	if !result.OK {
		summary := strings.TrimSpace(result.Summary)
		if summary == "" {
			summary = "计划管理未成功"
		}
		return agentdispatch.PartialFailure(summary, "", result.Handles)
	}
	summary := strings.TrimSpace(result.Summary)
	if result.Action == "query" {
		if text, ok := result.Detail["result"].(string); ok && strings.TrimSpace(text) != "" {
			summary = text
		}
	}
	if summary == "" {
		summary = "计划管理已完成"
	}
	return agentdispatch.Success(summary, result.Handles)
}

// recentActivityResult 按需读取最近生活记录（不再由运行时消息主动注入）。
func recentActivityResult() agentdispatch.Result {
	if LifeSnapshot == nil {
		return agentdispatch.Failure("生活链未接入，无法读取最近活动", nil)
	}
	lastRecord, activeEvent := LifeSnapshot()
	parts := make([]string, 0, 2)
	if text := strings.TrimSpace(lastRecord); text != "" {
		parts = append(parts, "最近做过："+text)
	}
	if text := strings.TrimSpace(activeEvent); text != "" {
		parts = append(parts, "有件事正在进行中")
	}
	if len(parts) == 0 {
		return agentdispatch.Success("最近没有生活记录", nil)
	}
	return agentdispatch.Success(strings.Join(parts, "；"), nil)
}

// recordCreativeIntent 覆盖生活创作的参考字符串。
func recordCreativeIntent(instruction string) agentdispatch.Result {
	text := strings.TrimSpace(instruction)
	if text == "" {
		return agentdispatch.Failure("创作意图不能为空", nil)
	}
	if SetCreativeIntent == nil {
		return agentdispatch.Failure("生活链未接入，无法记录创作意图", nil)
	}
	if err := SetCreativeIntent(text); err != nil {
		return agentdispatch.Failure("记录创作意图失败："+err.Error(), nil)
	}
	return agentdispatch.Success("已记录下一轮创作意图："+text, nil)
}

// lifePlanningScope 各分支职能描述（details=true 专用）。
func lifePlanningScope(kind string) (string, bool) {
	switch kind {
	case LifePlanningPlanManager:
		return "计划管理子智能体：按自然语言新增、删除、改写与查询计划表；可以先查看目标时间段再操作。", true
	case LifePlanningRecentActivity:
		return "最近活动：读取生活系统维护的最近生活记录与进行中的事件。", true
	case LifePlanningRecordIntent:
		return "创作意图：用对话中的描述覆盖生活创作的参考字符串，下一轮生活创作读取后执行。", true
	}
	return "", false
}
