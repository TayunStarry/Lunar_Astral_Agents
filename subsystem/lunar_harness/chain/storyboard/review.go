package storyboard

// review.go 审查态与任务控制：确认审查稿、暂停、继续、终止。
//
// 设计约束：
//   - 未确认不绘制：只有 ConfirmTask 能把任务从 reviewing 推进到 revising；
//   - 终止立刻生效：置标记并取消当前步骤上下文，已产出镜头原样保留；
//   - 续跑不重画：暂停绘制只把当前镜回退为待绘制，已完成镜头不动。

import (
	"fmt"
	"strings"
	"time"
)

// ConfirmTask 用用户审查后的分镜替换规划稿，并进入修订阶段。
func ConfirmTask(taskID, title string, input []ReviewShot) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return TaskView{}, fmt.Errorf("缺少任务标识")
	}
	if len(input) == 0 {
		return TaskView{}, fmt.Errorf("至少保留一个分镜")
	}
	if len(input) > maxReviewShots {
		return TaskView{}, fmt.Errorf("分镜数量 %d 超过上限 %d", len(input), maxReviewShots)
	}

	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务不存在")
	}
	if state.stage != stageReviewing || state.paused || state.canceled {
		status := state.view.Status
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("当前状态不可确认分镜：%s", status)
	}

	style := strings.TrimSpace(state.req.Style)
	ratio := strings.TrimSpace(state.req.Ratio)
	if ratio == "" {
		ratio = defaultRatio
	}
	shots := make([]ShotView, 0, len(input))
	planShots := make([]plannedShot, 0, len(input))
	for index, item := range input {
		act := strings.TrimSpace(item.Act)
		scene := strings.TrimSpace(item.Scene)
		if scene == "" {
			scene = fmt.Sprintf("镜 %d", index+1)
		}
		narration := strings.TrimSpace(item.Narration)
		shotRatio := strings.TrimSpace(item.Ratio)
		if shotRatio == "" {
			shotRatio = ratio
		}
		// 画面描述留空交给「分镜规划-修订」补全；用户已写的内容原样保留。
		prompt := clipRunes(strings.TrimSpace(item.Prompt), maxPaintPromptRunes)
		shots = append(shots, ShotView{
			Index:     index + 1,
			Act:       act,
			Scene:     scene,
			Narration: narration,
			Prompt:    prompt,
			Ratio:     shotRatio,
			Status:    shotPlanned,
		})
		planShots = append(planShots, plannedShot{
			Index:     index + 1,
			Act:       act,
			Scene:     scene,
			Narration: narration,
			Prompt:    prompt,
			Ratio:     shotRatio,
			Style:     style,
		})
	}

	viewTitle := clipRunes(strings.TrimSpace(title), 60)
	if viewTitle == "" {
		viewTitle = state.plan.Title
	}

	state.plan = storyboardPlan{Title: viewTitle, Style: style, Shots: planShots}
	state.shots = shots
	state.nextShot = 0
	state.previous = ""
	state.stage = stageRevising
	releaseDeadlineLocked(state)
	applyEventLocked(state, ProgressEvent{
		Status:   statusRevising,
		Stage:    "审查已确认，正在梳理分镜衔接",
		Progress: 12,
		Summary:  "分镜修订完成后开始逐镜绘制",
		Title:    viewTitle,
		Event:    eventReviewConfirmed,
		Shots:    cloneShots(shots),
	})
	view := takeViewLocked(state)
	persistTasksLocked()
	taskMu.Unlock()
	emitTask(view)
	return view, nil
}

// PauseTask 暂停任务：绘制阶段保留已完成镜头，未完成镜头回到待绘制。
func PauseTask(taskID string) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务不存在")
	}
	if state.stage == stageDone || state.canceled {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务已结束，无法暂停")
	}
	if state.paused {
		view := cloneTaskView(state)
		taskMu.Unlock()
		return view, nil
	}
	switch state.stage {
	case stageReviewing:
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("审查态本身不占用队列，直接确认或终止即可")
	case stageWriting:
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("文档生成中，无法暂停")
	}

	state.paused = true
	if state.restedAt.IsZero() {
		state.restedAt = time.Now()
	}
	// 当前正在绘制的镜头回退为待绘制，续跑时从这一镜继续。
	for index := range state.shots {
		if state.shots[index].Status == shotDrawing {
			state.shots[index].Status = shotPlanned
			state.shots[index].Error = ""
		}
	}
	if state.stage == stagePlanning {
		// 规划尚未定稿：恢复时重新推演，避免半份分镜与新推演交错。
		state.plan = storyboardPlan{}
		state.shots = nil
		state.nextShot = 0
	}
	stageText := "已暂停"
	if state.stage == stagePainting {
		stageText = fmt.Sprintf("已暂停，可从第 %d 镜继续", state.nextShot+1)
	}
	if state.stage == stageRevising {
		stageText = "已暂停，继续后重新整理分镜"
	}
	applyEventLocked(state, ProgressEvent{
		Status: statusPaused,
		Stage:  stageText,
		Event:  eventPaused,
		Shots:  cloneShots(state.shots),
	})
	cancel := state.cancel
	state.cancel = nil
	view := takeViewLocked(state)
	persistTasksLocked()
	taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	emitTask(view)
	return view, nil
}

// ResumeTask 继续一个已暂停的任务；绘制阶段从下一个未完成镜头接着画。
func ResumeTask(taskID string) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务不存在")
	}
	if !state.paused {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务当前未暂停")
	}
	state.paused = false
	releaseDeadlineLocked(state)
	event := ProgressEvent{
		Status: stageStatus(state.stage),
		Stage:  "继续执行",
		Event:  eventResumed,
	}
	switch state.stage {
	case stagePlanning:
		// 规划重新开始：清空过程文本与半成品分镜，前端同步清屏。
		event.ThoughtReset = true
		event.Stage = "继续推演故事并拆分分镜"
		state.plan = storyboardPlan{}
		state.shots = nil
		state.nextShot = 0
	case stagePainting:
		event.Stage = fmt.Sprintf("继续绘制第 %d / %d 镜", state.nextShot+1, len(state.plan.Shots))
	case stageRevising:
		event.Stage = "继续整理审查后的分镜"
	}
	applyEventLocked(state, event)
	view := takeViewLocked(state)
	persistTasksLocked()
	taskMu.Unlock()
	emitTask(view)
	return view, nil
}

// CancelTask 终止任务：立刻取消当前步骤，已产出镜头保留在任务视图中。
func CancelTask(taskID, reason string) (TaskView, error) {
	taskID = strings.TrimSpace(taskID)
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok {
		taskMu.Unlock()
		return TaskView{}, fmt.Errorf("任务不存在")
	}
	if state.stage == stageDone {
		view := cloneTaskView(state)
		taskMu.Unlock()
		return view, nil
	}
	reason = clipRunes(strings.TrimSpace(reason), maxErrorRunes)
	if reason == "" {
		reason = "用户终止了演绎任务"
	}
	state.canceled = true
	state.paused = false
	state.stage = stageDone
	for index := range state.shots {
		if state.shots[index].Status == shotDrawing {
			state.shots[index].Status = shotPlanned
		}
	}
	applyEventLocked(state, ProgressEvent{
		Status:  statusCanceled,
		Stage:   "已终止",
		Summary: reason,
		Event:   eventCanceled,
		Shots:   cloneShots(state.shots),
	})
	cancel := state.cancel
	state.cancel = nil
	view := takeViewLocked(state)
	persistTasksLocked()
	taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
	emitTask(view)
	return view, nil
}

// releaseDeadlineLocked 把暂停/审查期间的时间顺延到任务截止时间；调用方必须持有锁。
func releaseDeadlineLocked(state *taskState) {
	if state.restedAt.IsZero() {
		return
	}
	if !state.deadline.IsZero() {
		state.deadline = state.deadline.Add(time.Since(state.restedAt))
	}
	state.restedAt = time.Time{}
}

// stageStatus 把内部阶段映射为对外状态。
func stageStatus(stage string) string {
	switch stage {
	case stageResearching:
		return statusResearching
	case stagePlanning:
		return statusPlanning
	case stageReviewing:
		return statusReviewing
	case stageRevising:
		return statusRevising
	case stagePainting:
		return statusPainting
	case stageWriting:
		return statusWriting
	default:
		return statusQueued
	}
}
