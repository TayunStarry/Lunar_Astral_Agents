package storyboard

// advance.go 演绎步骤机：调度器每个 Tick 只推进一个原子步骤。
//
// 步骤划分：
//  1. queued      → 读取故事正文并校验字数；
//  2. researching → 调用「分镜规划-资料」生成检索意图，每 Tick 执行一条网络搜索；
//  3. planning    → 调用「分镜规划-拆解」推演故事并逐镜拆分；
//  4. reviewing   → 等待用户审查确认（不占用 Tick）；
//  5. revising    → 调用「分镜规划-修订」整理用户确认后的最终稿；
//  6. painting    → 每次只绘制一镜，镜头之间立即让出 Tick；
//  7. writing     → 迁移图片并写出分镜文档；
//  8. done        → 任务结束，不再参与调度。
//
// 所有阶段推进都通过增量事件推送，前端据此逐条追加渲染。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/StoryboardPlanner"
	"LunarSubsystem/LunarHarness/agents/StoryboardReviser"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// Advance 推进最早一个可推进任务的单个原子步骤。
func Advance(ctx context.Context) error {
	taskMu.Lock()
	state := oldestActionableLocked()
	if state == nil {
		taskMu.Unlock()
		return nil
	}
	taskID := state.view.TaskID
	stage := state.stage
	expired := !state.deadline.IsZero() && !time.Now().Before(state.deadline)
	taskMu.Unlock()

	if expired {
		failTask(taskID, fmt.Errorf("任务执行超时（超过 %v）", runTimeout))
		return nil
	}

	switch stage {
	case stageQueued:
		advanceQueued(taskID)
	case stageResearching:
		advanceResearching(ctx, taskID)
	case stagePlanning:
		advancePlanning(ctx, taskID)
	case stageRevising:
		advanceRevising(ctx, taskID)
	case stagePainting:
		advancePainting(ctx, taskID)
	case stageWriting:
		advanceWriting(taskID)
	}
	return nil
}

// advanceQueued 读取故事正文并进入资料准备阶段。
func advanceQueued(taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stageQueued || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	req := state.req
	taskMu.Unlock()

	story, err := resolveStory(req)
	if err != nil {
		failTask(taskID, err)
		return
	}

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stageQueued || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	state.story = story
	state.stage = stageResearching
	applyEventLocked(state, ProgressEvent{
		Status:   statusResearching,
		Stage:    "正在准备资料",
		Progress: 2,
		Summary:  "已读取故事，开始补全设定资料",
		Event:    eventResearchStarted,
	})
	commitLocked(state)
}

// advancePlanning 调用「分镜规划-拆解」智能体完成推演与拆分，
// 完成后进入审查态等待用户确认。
func advancePlanning(ctx context.Context, taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stagePlanning || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	story := state.story
	req := state.req
	notes := strings.Join(state.research.notes, "\n\n")
	stepCtx, cancel := taskStepContext(ctx, state, stagePlanTimeout())
	state.cancel = cancel
	taskMu.Unlock()
	defer cancel()

	// 可读推演文本逐段推送：让用户在规划阶段就看到主智能体在分析什么。
	onThought := func(text string) {
		if text == "" {
			return
		}
		taskMu.Lock()
		state, ok := tasks[taskID]
		if !ok || state.stage != stagePlanning || state.paused || state.canceled {
			taskMu.Unlock()
			return
		}
		applyEventLocked(state, ProgressEvent{
			Status:       statusPlanning,
			Progress:     5,
			Event:        eventThinking,
			ThoughtDelta: text,
		})
		commitVolatileLocked(state)
	}

	// 每解析出一镜立即写入任务视图并推送 shot_planned，
	// 前端据此逐条追加渲染，而不是等整份分镜规划完再刷新。
	titleOverride := clipRunes(strings.TrimSpace(req.Title), 60)
	streamed := make([]ShotView, 0, 16)
	onShot := func(title string, shot StoryboardPlanner.Shot) {
		streamed = append(streamed, shotViewFromPlanner(shot, shotPending))
		viewTitle := titleOverride
		if viewTitle == "" {
			viewTitle = clipRunes(title, 60)
		}
		taskMu.Lock()
		state, ok := tasks[taskID]
		if !ok || state.stage != stagePlanning || state.paused || state.canceled {
			taskMu.Unlock()
			return
		}
		state.plan.Title = viewTitle
		state.shots = cloneShots(streamed)
		applyEventLocked(state, ProgressEvent{
			Status:    statusPlanning,
			Stage:     fmt.Sprintf("正在推演第 %d 镜（%s）", shot.Index, shot.Scene),
			Progress:  planningPercent(shot.Index),
			Title:     viewTitle,
			Event:     eventShotPlanned,
			ShotIndex: shot.Index,
			Shots:     cloneShots(streamed),
		})
		commitVolatileLocked(state)
	}

	agentPlan, err := StoryboardPlanner.Plan(stepCtx, StoryboardPlanner.Request{
		Story:     story,
		Style:     req.Style,
		Ratio:     req.Ratio,
		Research:  notes,
		OnThought: onThought,
		OnShot:    onShot,
	})
	if err != nil {
		// 暂停/终止已经由操作接口写入终态，这里不再覆盖。
		if isPausedOrCanceled(taskID) {
			return
		}
		failTask(taskID, err)
		return
	}
	plan := toStoryboardPlan(agentPlan)
	if titleOverride != "" {
		plan.Title = titleOverride
	}
	shots := shotsFromPlan(plan, shotPending)

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stagePlanning || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	state.plan = plan
	state.shots = shots
	state.nextShot = 0
	state.previous = ""
	state.stage = stageReviewing
	state.cancel = nil
	state.restedAt = time.Now()
	applyEventLocked(state, ProgressEvent{
		Status:   statusReviewing,
		Stage:    fmt.Sprintf("分镜已生成，共 %d 镜，等待你的审查确认", len(shots)),
		Progress: 12,
		Summary:  "审查确认后开始逐镜绘制",
		Title:    plan.Title,
		Event:    eventReviewReady,
		Shots:    cloneShots(shots),
	})
	commitLocked(state)
}

// advanceRevising 调用「分镜规划-修订」整理审查稿，完成后进入逐镜绘制。
//
// 修订失败不阻断绘制：按用户确认稿原样进入绘制，避免用户内容被卡住。
func advanceRevising(ctx context.Context, taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stageRevising || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	req := state.req
	title := state.plan.Title
	shots := reviserShots(state.plan.Shots)
	stepCtx, cancel := taskStepContext(ctx, state, stageReviseTimeout())
	state.cancel = cancel
	taskMu.Unlock()
	defer cancel()

	result, err := StoryboardReviser.Revise(stepCtx, StoryboardReviser.Request{
		Title: title,
		Style: req.Style,
		Ratio: req.Ratio,
		Shots: shots,
	})

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stageRevising || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	state.cancel = nil
	summary := "审查已确认，开始逐镜绘制"
	if err != nil {
		logger.Warn(Module, "演绎任务 %s 修订分镜失败，按审查稿直接绘制: %v", taskID, err)
		summary = "分镜修订失败，按审查稿直接绘制"
	} else {
		state.plan.Title = result.Title
		state.plan.Shots = plannedShotsFromReviser(result.Shots, req.Style)
	}
	// 修订未补全或修订失败时，用分镜叙事兜底出可绘制的画面描述。
	state.plan.Shots = fillMissingPrompts(state.plan.Shots)
	state.shots = shotsFromPlan(state.plan, shotPlanned)
	state.nextShot = 0
	state.previous = ""
	state.stage = stagePainting
	applyEventLocked(state, ProgressEvent{
		Status:   statusPainting,
		Stage:    "审查已确认，开始逐镜绘制",
		Progress: 12,
		Summary:  summary,
		Title:    state.plan.Title,
		Event:    eventRevising,
		Shots:    cloneShots(state.shots),
	})
	commitLocked(state)
}

// advancePainting 绘制单镜：先推送开镜事件，再执行绘制，最后推送结果事件。
func advancePainting(ctx context.Context, taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stagePainting || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	total := len(state.plan.Shots)
	if state.nextShot >= total {
		state.stage = stageWriting
		applyEventLocked(state, ProgressEvent{
			Status:   statusWriting,
			Stage:    "正在迁移分镜图片并生成文档",
			Progress: 90,
			Event:    eventWriting,
			Shots:    cloneShots(state.shots),
		})
		commitLocked(state)
		return
	}

	index := state.nextShot
	shot := state.plan.Shots[index]
	previous := state.previous
	state.shots[index].Status = shotDrawing
	state.shots[index].Error = ""
	applyEventLocked(state, ProgressEvent{
		Status:    statusPainting,
		Stage:     fmt.Sprintf("正在绘制第 %d / %d 镜（%s）", index+1, total, shot.Scene),
		Progress:  paintingPercent(index, total),
		Event:     eventShotStarted,
		ShotIndex: index + 1,
		Shots:     cloneShots(state.shots),
	})
	stepCtx, cancel := taskStepContext(ctx, state, stagePaintTimeout())
	state.cancel = cancel
	started := takeViewLocked(state)
	taskMu.Unlock()
	emitTask(started)
	defer cancel()

	result, paintErr := paintShot(stepCtx, shot, previous)

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stagePainting || state.nextShot != index {
		taskMu.Unlock()
		return
	}
	state.cancel = nil
	// 终止优先级最高：操作接口已经写好终态，这里只保留已产出结果。
	if state.canceled {
		if paintErr == nil && len(result.Handles) > 0 {
			state.shots[index].Status = shotPainted
			state.shots[index].Handle = result.Handles[0]
			state.view.Shots = cloneShots(state.shots)
			persistTasksLocked()
		}
		taskMu.Unlock()
		return
	}
	// 暂停：成功的镜头照常保留，失败/被取消的镜头回到待绘制，续跑时重画这一镜。
	if state.paused {
		if paintErr == nil && len(result.Handles) > 0 {
			state.shots[index].Status = shotPainted
			state.shots[index].Handle = result.Handles[0]
			state.previous = result.Handles[0]
			state.nextShot = index + 1
			applyEventLocked(state, ProgressEvent{
				Status:    statusPaused,
				Progress:  paintingPercent(index+1, total),
				Event:     eventShotCompleted,
				ShotIndex: index + 1,
				Shots:     cloneShots(state.shots),
			})
		} else {
			state.shots[index].Status = shotPlanned
			state.shots[index].Error = ""
			state.view.Shots = cloneShots(state.shots)
		}
		commitVolatileLocked(state)
		return
	}

	event := ProgressEvent{
		Status:    statusPainting,
		Progress:  paintingPercent(index+1, total),
		ShotIndex: index + 1,
	}
	if paintErr != nil {
		state.shots[index].Status = shotFailed
		state.shots[index].Error = clipRunes(paintErr.Error(), maxErrorRunes)
		event.Stage = fmt.Sprintf("第 %d / %d 镜绘制失败（%s）", index+1, total, shot.Scene)
		event.Summary = state.shots[index].Error
		event.Event = eventShotFailed
	} else {
		state.shots[index].Status = shotPainted
		state.shots[index].Handle = result.Handles[0]
		state.previous = result.Handles[0]
		event.Stage = fmt.Sprintf("已完成第 %d / %d 镜（%s）", index+1, total, shot.Scene)
		event.Event = eventShotCompleted
	}
	state.nextShot = index + 1
	event.Shots = cloneShots(state.shots)
	applyEventLocked(state, event)
	commitVolatileLocked(state)
}

// advanceWriting 迁移图片、生成分镜文档并收尾。
func advanceWriting(taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stageWriting || state.canceled || state.paused {
		taskMu.Unlock()
		return
	}
	plan := state.plan
	shots := cloneShots(state.shots)
	taskMu.Unlock()

	doc, updated, err := writeStoryboardDoc(plan, shots)
	if err != nil {
		if isPausedOrCanceled(taskID) {
			return
		}
		failTask(taskID, err)
		return
	}
	success, failed := countShots(updated)
	handles := make([]string, 0, success+1)
	for _, shot := range updated {
		if shot.Status == shotPainted && shot.Handle != "" {
			handles = append(handles, shot.Handle)
		}
	}
	if doc.Handle != "" {
		handles = append(handles, doc.Handle)
	}
	summary := fmt.Sprintf("分镜 %d 镜：成功 %d，失败 %d；文档 %s", len(updated), success, failed, doc.Path)

	status := statusCompleted
	stage := "演绎完成"
	event := eventCompleted
	failure := ""
	if success == 0 {
		status = statusFailed
		stage = "全部镜头绘制失败"
		event = eventFailed
		failure = summary
	}

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stageWriting {
		taskMu.Unlock()
		return
	}
	state.stage = stageDone
	state.shots = updated
	applyEventLocked(state, ProgressEvent{
		Status:    status,
		Stage:     stage,
		Progress:  100,
		Summary:   summary,
		Title:     plan.Title,
		Event:     event,
		DocPath:   doc.Path,
		DocHandle: doc.Handle,
		Shots:     cloneShots(updated),
		Handles:   handles,
	})
	if failure != "" {
		state.view.Error = failure
	}
	commitLocked(state)
}

// failTask 把任务标记为失败并推送终止事件。
func failTask(taskID string, err error) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage == stageDone || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	reason := clipRunes(strings.TrimSpace(err.Error()), maxErrorRunes)
	state.stage = stageDone
	applyEventLocked(state, ProgressEvent{
		Status:   statusFailed,
		Stage:    "执行失败",
		Progress: 100,
		Summary:  reason,
		Event:    eventFailed,
		Shots:    cloneShots(state.shots),
	})
	state.view.Error = reason
	commitLocked(state)
	logger.Warn(Module, "演绎任务 %s 失败: %s", taskID, reason)
}

// isPausedOrCanceled 报告任务是否已被用户暂停或终止。
func isPausedOrCanceled(taskID string) bool {
	taskMu.Lock()
	defer taskMu.Unlock()
	state, ok := tasks[taskID]
	return ok && (state.paused || state.canceled)
}

// taskStepContext 单步超时：不超过任务总截止时间。
func taskStepContext(ctx context.Context, state *taskState, step time.Duration) (context.Context, context.CancelFunc) {
	if !state.deadline.IsZero() {
		if remaining := time.Until(state.deadline); remaining < step {
			if remaining <= 0 {
				remaining = time.Second
			}
			step = remaining
		}
	}
	return context.WithTimeout(ctx, step)
}
