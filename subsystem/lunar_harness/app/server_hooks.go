package app

// server_hooks.go 装配层向 HTTP 层注入跨层能力。
//
// server 只声明函数变量（见 server/variable.go），由这里把编排层与智能体实现接上，
// 保证 HTTP 层不 import chain/* 与 agents/Responder。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"LunarSubsystem/LunarHarness/agents/Responder"
	"LunarSubsystem/LunarHarness/agents/SQLWriter"
	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/chain/thinking"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	"LunarSubsystem/LunarHarness/server"
)

// wireServerHooks 在 server.Start 之前完成全部注入。
func wireServerHooks() {
	server.MessageBatchHandlerFunc = thinking.MessageBatchHandler
	server.VisionReadFunc = thinking.VisionReadBlob
	server.AgentPendingFunc = Responder.Pending
	server.AgentEnqueueFunc = enqueueExternalAgent
	server.AgentPrepareMessagesFunc = prepareAgentMessages
	// 资源库自然语言转 SQL：HTTP 层只收参，语句生成由 SQLWriter 智能体完成。
	server.DBSQLGenerateFunc = SQLWriter.Generate
	// 资源库创作入口：HTTP 层只收参，规划执行由创作链完成。
	server.LibraryCreationSubmitFunc = submitLibraryCreation
	server.LibraryCreationStatusFunc = libraryCreationStatus
	server.LibraryCreationListFunc = libraryCreationList
	// 资源库演绎入口：HTTP 层只收参，分镜规划与逐镜绘制由演绎链完成。
	server.LibraryStoryboardSubmitFunc = submitLibraryStoryboard
	server.LibraryStoryboardStatusFunc = libraryStoryboardStatus
	server.LibraryStoryboardListFunc = libraryStoryboardList
	// 演绎任务控制：审查确认、暂停、继续、终止。
	server.LibraryStoryboardConfirmFunc = confirmLibraryStoryboard
	server.LibraryStoryboardPauseFunc = pauseLibraryStoryboard
	server.LibraryStoryboardResumeFunc = resumeLibraryStoryboard
	server.LibraryStoryboardCancelFunc = cancelLibraryStoryboard
}

// submitLibraryStoryboard 把资源库请求转成演绎任务。
func submitLibraryStoryboard(req server.LibraryStoryboardRequest) (server.LibraryStoryboardTask, error) {
	view, err := storyboard.SubmitTask(storyboard.SubmitRequest{
		Text:           req.Text,
		Handle:         req.Handle,
		Title:          req.Title,
		Style:          req.Style,
		Ratio:          req.Ratio,
		ReviewRequired: true,
	})
	return toLibraryStoryboardTask(view), err
}

// libraryStoryboardStatus 查询单个演绎任务。
func libraryStoryboardStatus(taskID string) (server.LibraryStoryboardTask, bool) {
	view, ok := storyboard.TaskStatus(taskID)
	if !ok {
		return server.LibraryStoryboardTask{}, false
	}
	return toLibraryStoryboardTask(view), true
}

// libraryStoryboardList 查询最近演绎任务。
func libraryStoryboardList(limit int) []server.LibraryStoryboardTask {
	views := storyboard.TaskList(limit)
	out := make([]server.LibraryStoryboardTask, 0, len(views))
	for _, view := range views {
		out = append(out, toLibraryStoryboardTask(view))
	}
	return out
}

// confirmLibraryStoryboard 把审查稿转成演绎链的确认请求。
func confirmLibraryStoryboard(taskID, title string, shots []server.LibraryStoryboardShotInput) (server.LibraryStoryboardTask, error) {
	input := make([]storyboard.ReviewShot, 0, len(shots))
	for _, shot := range shots {
		input = append(input, storyboard.ReviewShot{
			Index:     shot.Index,
			Act:       shot.Act,
			Scene:     shot.Scene,
			Narration: shot.Narration,
			Prompt:    shot.Prompt,
			Ratio:     shot.Ratio,
		})
	}
	view, err := storyboard.ConfirmTask(taskID, title, input)
	return toLibraryStoryboardTask(view), err
}

// pauseLibraryStoryboard 暂停演绎任务。
func pauseLibraryStoryboard(taskID string) (server.LibraryStoryboardTask, error) {
	view, err := storyboard.PauseTask(taskID)
	return toLibraryStoryboardTask(view), err
}

// resumeLibraryStoryboard 继续已暂停的演绎任务。
func resumeLibraryStoryboard(taskID string) (server.LibraryStoryboardTask, error) {
	view, err := storyboard.ResumeTask(taskID)
	return toLibraryStoryboardTask(view), err
}

// cancelLibraryStoryboard 终止演绎任务。
func cancelLibraryStoryboard(taskID, reason string) (server.LibraryStoryboardTask, error) {
	view, err := storyboard.CancelTask(taskID, reason)
	return toLibraryStoryboardTask(view), err
}

// toLibraryStoryboardTask 把演绎链任务视图映射为 HTTP 层结构。
func toLibraryStoryboardTask(view storyboard.TaskView) server.LibraryStoryboardTask {
	return server.LibraryStoryboardTask{
		TaskID:    view.TaskID,
		Status:    view.Status,
		Stage:     view.Stage,
		Progress:  view.Progress,
		Summary:   view.Summary,
		Title:     view.Title,
		DocPath:   view.DocPath,
		DocHandle: view.DocHandle,
		Shots:     toLibraryStoryboardShots(view.Shots),
		ShotPatch: toLibraryStoryboardShots(view.ShotPatch),
		ShotCount: view.ShotCount,
		Handles:   append([]string(nil), view.Handles...),
		Error:     view.Error,
		Event:     view.Event,
		ShotIndex: view.ShotIndex,
		Thought:   view.Thought,
		Seq:       view.Seq,
		Epoch:     view.Epoch,
		V:         view.V,
		CreatedAt: view.CreatedAt,
		UpdatedAt: view.UpdatedAt,
	}
}

// toLibraryStoryboardShots 映射镜头列表；空列表保持为空，避免推送出现多余的空数组。
func toLibraryStoryboardShots(source []storyboard.ShotView) []server.LibraryStoryboardShot {
	if len(source) == 0 {
		return nil
	}
	out := make([]server.LibraryStoryboardShot, 0, len(source))
	for _, shot := range source {
		out = append(out, server.LibraryStoryboardShot{
			Index:     shot.Index,
			Act:       shot.Act,
			Scene:     shot.Scene,
			Narration: shot.Narration,
			Prompt:    shot.Prompt,
			Ratio:     shot.Ratio,
			Status:    shot.Status,
			Handle:    shot.Handle,
			File:      shot.File,
			Error:     shot.Error,
		})
	}
	return out
}

// agentEnvelopeTypes 需要经过信封接收链路的内容类型（六类新信封 + 四类已废止旧信封）。
// 命旧类型时同样进入链路：由接收侧判定无效并记录丢弃原因。
var agentEnvelopeTypes = map[string]bool{
	mediaobject.EnvelopeImageStatic:   true,
	mediaobject.EnvelopeImageAnimated: true,
	mediaobject.EnvelopeVideo:         true,
	mediaobject.EnvelopeAudio:         true,
	mediaobject.EnvelopeFile:          true,
	mediaobject.EnvelopeWebpage:       true,
	mediaobject.EnvelopeImageURL:      true,
	mediaobject.EnvelopeVideoURL:      true,
	mediaobject.EnvelopeAudioURL:      true,
	mediaobject.EnvelopeInputAudio:    true,
}

// prepareAgentMessages 把外部推理请求里的多模态内容送进统一信封链路。
//
// 纯文本/工具消息原样返回，避免无谓的 JSON 重编码丢失调用方字段；
// 只有出现媒体内容项时才解析为 PostMessage 并做「校验 → 落盘 → 清洗」。
// 结构非法时直接报错：无法判断是否含媒体就放行，等于让请求绕过信封协议。
func prepareAgentMessages(raw json.RawMessage) (json.RawMessage, error) {
	hasMedia, err := hasAgentEnvelopeContent(raw)
	if err != nil {
		return nil, err
	}
	if !hasMedia {
		return raw, nil
	}
	var messages []thinking.PostMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("messages 解析失败: %v", err)
	}
	converted, err := json.Marshal(thinking.ResourcesConversionHandle(messages))
	if err != nil {
		return nil, fmt.Errorf("messages 清洗失败: %v", err)
	}
	return converted, nil
}

// hasAgentEnvelopeContent 判断消息数组里是否含媒体内容项。
//
// content 的合法形态只有三种：缺省、null、字符串（纯文本），或内容对象数组。
// 其余形态（数字/布尔/裸对象、数组里出现非对象元素）无法判定是否含媒体，
// 一律返回错误，交给上层回 400，不再静默透传。
func hasAgentEnvelopeContent(raw json.RawMessage) (bool, error) {
	var messages []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &messages); err != nil {
		return false, fmt.Errorf("messages 结构无效（应为消息对象数组）: %v", err)
	}
	if len(messages) == 0 {
		return false, errors.New("messages 不能为空")
	}
	for index, message := range messages {
		hasMedia, err := messageHasEnvelope(message.Content)
		if err != nil {
			return false, fmt.Errorf("第 %d 条消息的 content 非法: %v", index+1, err)
		}
		if hasMedia {
			return true, nil
		}
	}
	return false, nil
}

// messageHasEnvelope 检查单条消息的 content 是否含媒体内容项。
func messageHasEnvelope(content json.RawMessage) (bool, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false, nil
	}
	if trimmed[0] == '"' {
		return false, nil
	}
	if trimmed[0] != '[' {
		return false, errors.New("只允许字符串、null 或内容对象数组")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return false, fmt.Errorf("不是合法数组: %v", err)
	}
	for index, item := range items {
		entry := bytes.TrimSpace(item)
		if len(entry) == 0 || entry[0] != '{' {
			return false, fmt.Errorf("第 %d 项不是内容对象", index+1)
		}
		var item struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(entry, &item); err != nil {
			return false, fmt.Errorf("第 %d 项解析失败: %v", index+1, err)
		}
		if agentEnvelopeTypes[item.Type] {
			return true, nil
		}
	}
	return false, nil
}

// enqueueExternalAgent 把 server 的排队请求适配为响应者智能体的入队请求，
// 并把异步回执转回 HTTP 层使用的结构（两者字段一致，仅类型不同）。
func enqueueExternalAgent(req *server.AgentRequest) error {
	inner := &Responder.Request{
		Messages: req.Messages,
		Params:   req.Params,
		Reply:    make(chan Responder.Result, 1),
	}
	if err := Responder.Enqueue(inner); err != nil {
		return err
	}
	go func() {
		result := <-inner.Reply
		converted := server.AgentResult{
			Content:      result.Content,
			Reasoning:    result.Reasoning,
			ToolCalls:    result.ToolCalls,
			FinishReason: result.FinishReason,
			Err:          result.Err,
		}
		// 请求超时后 handler 已返回，此时丢弃结果即可（缓冲为 1，正常路径不会阻塞）
		select {
		case req.Reply <- converted:
		default:
		}
	}()
	return nil
}
