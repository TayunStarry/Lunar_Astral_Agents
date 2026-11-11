package Dialogue

// tool_life_planning.go 生活规划聚合工具执行器：愿望、演绎、文件操作、观影与论坛。
//
// 统一输入 {type, instruction, handle, details}，输出 {result, handle, error}。
// 愿望类指令直接交给愿望管理智能体；文件操作由人格对话直接驱动并同步取回结果；
// 演绎 / 观影 / 论坛仍登记强制生活事件，下一 Tick 由生活链执行。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/WishManager"
	"LunarSubsystem/LunarHarness/agents/file/FileOperations"
	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

const (
	// ToolLifePlanning 生活规划聚合工具名
	ToolLifePlanning = "life_planning"

	// lifeTypeWriteWish 记录 / 改写 / 删除愿望
	lifeTypeWriteWish = "write_wish"
	// lifeTypeWishComplete 标记愿望完成
	lifeTypeWishComplete = "wish_complete"
	// lifeTypeStoryboard 演绎：把故事拆成分镜并逐镜绘制
	lifeTypeStoryboard = "storyboard"
	// lifeTypeFileOperations 文件操作编排
	lifeTypeFileOperations = "file_operations"
	// lifeTypeBilibili 哔哩观影
	lifeTypeBilibili = "bilibili_viewer"
	// lifeTypeMutualForum 互助论坛
	lifeTypeMutualForum = "mutual_forum"

	// lifePlanningTimeout 愿望管理调用超时
	lifePlanningTimeout = 5 * time.Minute
	// fileOperationsTimeout 人格对话直接驱动文件操作时的总超时
	fileOperationsTimeout = 10 * time.Minute

	// StoryboardFixedReply 演绎触发后的固定响应
	StoryboardFixedReply = "月华这就把这段故事演绎成分镜"
)

// lifeTypeOrder 聚合工具 type 的固定顺序。
var lifeTypeOrder = []string{
	lifeTypeWriteWish, lifeTypeWishComplete,
	lifeTypeStoryboard, lifeTypeFileOperations,
	lifeTypeBilibili, lifeTypeMutualForum,
}

// lifeTypeScopes 各机能的职能描述（details=true 专用）。
var lifeTypeScopes = map[string]string{
	lifeTypeWriteWish:      "用自然语言描述你的愿望或什么时候想去做什么事",
	lifeTypeWishComplete:   "用自然语言描述你的什么愿望已达成",
	lifeTypeStoryboard:     "演绎子智能体：把故事拆成分镜并逐镜绘制，图片输出到多媒体标题目录，分镜文档写入 documents/lens/标题。",
	lifeTypeFileOperations: "文件操作子智能体：统一编排文件读写、文档整理、压缩包处理与脚本运行，按需调用文件操纵 / 归档操纵 / 代码执行 / 内容回滚子智能体。",
	lifeTypeBilibili:       "哔哩观影子智能体：登记看视频事件，由生活链播放一个待看片段、理解画面并在原视频评论区留下观感。",
	lifeTypeMutualForum:    "互助论坛子智能体：登记论坛事件，由生活链进入互助论坛流程。",
}

// lifeTypeDirectives 愿望类 type → 交给愿望管理智能体的操作提示。
var lifeTypeDirectives = map[string]string{
	lifeTypeWriteWish:    "【本轮操作类型】记录愿望：按自然语言自行判断是新增、改写、删除还是调整优先级。",
	lifeTypeWishComplete: "【本轮操作类型】愿望达成：把对应愿望标记为完成。",
}

// ExecuteLifePlanning 执行一次生活规划聚合工具调用。
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
	scope, ok := lifeTypeScopes[kind]
	if !ok {
		return toolJSON(agentdispatch.Failure("type 只能取 "+strings.Join(lifeTypeOrder, " / "), nil))
	}
	if call.Details {
		return toolJSON(agentdispatch.Success(scope, nil))
	}
	if call.Instruction == "" {
		return toolJSON(agentdispatch.Failure("instruction 不能为空，请用自然语言说明要做什么", nil))
	}

	switch kind {
	case lifeTypeWriteWish, lifeTypeWishComplete:
		return executeWishPlanning(kind, call.Instruction)
	case lifeTypeStoryboard:
		return scheduleLifePlanning(lifeTypeStoryboard, call.Instruction, call.Handle, StoryboardFixedReply)
	case lifeTypeFileOperations:
		return executeFileOperations(call.Instruction, call.Handle)
	case lifeTypeBilibili:
		return scheduleLifePlanning("watch", call.Instruction, call.Handle, WatchFixedReply)
	case lifeTypeMutualForum:
		return scheduleLifePlanning("forum", call.Instruction, call.Handle, ForumFixedReply)
	default:
		return toolJSON(agentdispatch.Failure("type 只能取 "+strings.Join(lifeTypeOrder, " / "), nil))
	}
}

// executeWishPlanning 把愿望类指令交给愿望管理智能体。
func executeWishPlanning(kind, instruction string) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), lifePlanningTimeout)
	defer cancel()
	outcome, err := WishManager.Execute(ctx, lifeTypeDirectives[kind]+"\n"+instruction)
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

// executeFileOperations 人格对话直接驱动文件操作智能体，并把实际结果作为工具结果返回。
func executeFileOperations(instruction string, handles []string) ToolResult {
	ctx, cancel := context.WithTimeout(context.Background(), fileOperationsTimeout)
	defer cancel()
	result, err := FileOperations.Run(ctx, FileOperations.Request{
		Instruction:   instruction,
		Handles:       append([]string(nil), handles...),
		WorkspaceRoot: "documents",
	})
	summary := strings.TrimSpace(result.Summary)
	if err != nil {
		if summary == "" {
			summary = err.Error()
		}
		return toolJSON(agentdispatch.PartialFailure("文件操作未完成："+err.Error(), summary, result.Handles))
	}
	if summary == "" {
		summary = "文件操作已完成"
	}
	return toolJSON(agentdispatch.Success(summary, result.Handles))
}

// scheduleLifePlanning 登记强制生活事件；下一 Tick 由生活链跳过随机选择直接执行。
func scheduleLifePlanning(eventID, instruction string, handles []string, reply string) ToolResult {
	if RequestLifeEvent == nil {
		return toolJSON(agentdispatch.Failure("生活链未接入，暂时无法执行该操作", nil))
	}
	if !RequestLifeEvent(eventID, instruction, handles) {
		return toolJSON(agentdispatch.Failure("生活事件登记失败，请稍后重试", nil))
	}
	return toolJSON(agentdispatch.Success(reply, nil))
}
