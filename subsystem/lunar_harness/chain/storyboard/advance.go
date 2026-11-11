package storyboard

// advance.go 演绎步骤机：调度器每个 Tick 只推进一个原子步骤。
//
// 步骤划分：
//  1. queued      → 读取故事正文并校验字数；
//  2. researching → 调用「分镜规划-资料」生成检索意图，每 Tick 执行一条网络搜索；
//  3. planning    → 调用「分镜规划-拆解」推演故事并逐镜拆分；
//  4. reviewing   → 仅手动提交的作品等待用户审查确认（不占用 Tick）；
//  5. painting    → 每次只绘制一镜，镜头之间立即让出 Tick；
//  6. writing     → 写出分镜文档并收尾；
//  7. done        → 任务结束，不再参与调度。
//
// 生活系统自动触发与月华命令触发的演绎跳过审查，规划完成即进入绘制。
// 所有阶段推进都通过增量事件推送，前端据此逐条追加渲染。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/StoryboardPlanner"
	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

// Advance 推进最早一个可推进任务的单个原子步骤。
//
// 超时与失败判定（与「运行总时长」彻底解耦）：
//   - 运行预算到期只做状态校验：仍在推进就顺延并提醒，绝不终止；
//   - 接近预算只提醒，不改变任务状态；
//   - 唯一会判失败的是「单个步骤超时后仍无任何状态更新」，
//     由每个原子步骤自己的兜底看门狗（beginStepGuard）负责。
//
// 注意：卡死判定故意不放在这里。调度器同一时刻只推进一个任务，
// 排队等待的时间不属于任何步骤，放在这里会把正常排队误判成卡死。
func Advance(ctx context.Context) error {
	taskMu.Lock()
	state := oldestActionableLocked()
	if state == nil {
		taskMu.Unlock()
		return nil
	}
	taskID := state.view.TaskID
	stage := state.stage
	stageText := state.view.Stage
	action := watchTaskLocked(state, time.Now())
	taskMu.Unlock()

	switch action {
	case watchdogRenew:
		renewBudget(taskID, stageText)
	case watchdogWarn:
		warnLongRunning(taskID, stageText)
	}

	switch stage {
	case stageQueued:
		advanceQueued(taskID)
	case stageResearching:
		advanceResearching(ctx, taskID)
	case stagePlanning:
		advancePlanning(ctx, taskID)
	case stagePainting:
		advancePainting(ctx, taskID)
	case stageWriting:
		advanceWriting(taskID)
	}
	return nil
}

// watchdogAction 看门狗对某个任务的处置（只会提醒或顺延，不会终止）。
type watchdogAction int

const (
	// watchdogIdle 无需干预
	watchdogIdle watchdogAction = iota
	// watchdogWarn 接近预算：只提醒
	watchdogWarn
	// watchdogRenew 预算到期：先校验，仍在推进则顺延并提醒
	watchdogRenew
)

// watchTaskLocked 判断任务是否需要预算处置；调用方必须持有 taskMu。
func watchTaskLocked(state *taskState, now time.Time) watchdogAction {
	if state.stage == stageDone || state.paused || state.canceled {
		return watchdogIdle
	}
	if !state.budgetAt.IsZero() && !now.Before(state.budgetAt) {
		return watchdogRenew
	}
	if !state.nextWarn.IsZero() && !now.Before(state.nextWarn) {
		return watchdogWarn
	}
	return watchdogIdle
}

// stallReason 生成卡死判定文案：必须说清「哪个阶段 + 静默了多久」。
func stallReason(stageText string, idle time.Duration) error {
	if stageText == "" {
		stageText = "未知阶段"
	}
	return fmt.Errorf("任务在「%s」阶段已 %s 没有任何状态更新，判定为执行卡死（自愈失败，请重新提交）",
		stageText, humanDuration(idle))
}

// renewBudget 预算到期的状态校验：先判断任务是否真的没有响应，再决定顺延还是判卡死。
func renewBudget(taskID, stageText string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage == stageDone || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	now := time.Now()
	if now.Before(state.budgetAt) {
		taskMu.Unlock()
		return
	}
	// 状态校验：静默超过上限说明确实没有响应，此时才允许判失败。
	idle := now.Sub(state.lastBeat)
	if !state.lastBeat.IsZero() && idle > runStallTimeout {
		taskMu.Unlock()
		failTask(taskID, stallReason(stageText, idle))
		return
	}
	running := now.Sub(state.startedAt)
	state.budgetAt = now.Add(runBudget)
	if state.nextWarn.Before(now.Add(runWarnInterval)) {
		state.nextWarn = now.Add(runWarnInterval)
	}
	applyEventLocked(state, ProgressEvent{
		Event:   eventLongRunning,
		Summary: fmt.Sprintf("任务已运行 %s，仍在正常推进（%s）；已自动顺延预算，不会被判定超时", humanDuration(running), stageText),
	})
	schedulePushLocked(state)
	taskMu.Unlock()
}

// warnLongRunning 接近预算的提前提醒：只提示，不改变任务状态。
func warnLongRunning(taskID, stageText string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage == stageDone || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	now := time.Now()
	if now.Before(state.nextWarn) {
		taskMu.Unlock()
		return
	}
	running := now.Sub(state.startedAt)
	state.nextWarn = now.Add(runWarnInterval)
	applyEventLocked(state, ProgressEvent{
		Event:   eventLongRunning,
		Summary: fmt.Sprintf("任务已运行 %s，接近预算上限，但仍在正常推进（%s）；不会中断，请继续等待", humanDuration(running), stageText),
	})
	schedulePushLocked(state)
	taskMu.Unlock()
}

// humanDuration 把时长格式化成可读文案，避免日志里出现 1h30m0s 这类原始写法。
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
	hours := int(d / time.Hour)
	minutes := int(d/time.Minute) % 60
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分", hours, minutes)
	}
	return fmt.Sprintf("%d 分钟", minutes)
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
	stopGuard := beginStepGuard(state, queuedStepTimeout)
	taskMu.Unlock()
	defer stopGuard()

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
	titleOverride := textutil.TruncateRunes(strings.TrimSpace(req.Title), 60)
	streamed := make([]ShotView, 0, 16)
	onShot := func(title string, shot StoryboardPlanner.Shot) {
		streamed = append(streamed, shotViewFromPlanner(shot, shotPending))
		viewTitle := titleOverride
		if viewTitle == "" {
			viewTitle = textutil.TruncateRunes(title, 60)
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
	// 画面描述缺失时先用分镜叙事兜底，保证可以直接进入绘制。
	plan.Shots = fillMissingPrompts(plan.Shots)
	state.plan = plan
	state.outputFolder = strings.TrimSpace(state.req.OutputFolder)
	if state.outputFolder == "" {
		state.outputFolder = prepareOutputFolder(plan.Title)
	}
	state.nextShot = 0
	state.cancel = nil
	if state.req.ReviewRequired {
		shots := shotsFromPlan(plan, shotPending)
		state.shots = shots
		state.stage = stageReviewing
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
		commitSnapshotLocked(state)
		return
	}
	// 生活系统自动触发与月华命令触发：跳过人工审查，直接开始分镜合成。
	shots := shotsFromPlan(plan, shotPlanned)
	state.shots = shots
	state.stage = stagePainting
	applyEventLocked(state, ProgressEvent{
		Status:   statusPainting,
		Stage:    fmt.Sprintf("分镜已生成，共 %d 镜，开始逐镜绘制", len(shots)),
		Progress: 12,
		Summary:  "无需人工审查，直接进入分镜合成",
		Title:    plan.Title,
		Event:    eventPlanCompleted,
		Shots:    cloneShots(shots),
	})
	commitSnapshotLocked(state)
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
			Stage:    "正在生成分镜文档",
			Progress: 90,
			Event:    eventWriting,
			Shots:    cloneShots(state.shots),
		})
		commitLocked(state)
		return
	}

	index := state.nextShot
	shot := state.plan.Shots[index]
	outputFolder := state.outputFolder
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
	// 开镜事件走合并推送：绘制本身可能长达数分钟，推送不能被绘制阻塞。
	schedulePushLocked(state)
	taskMu.Unlock()
	defer cancel()

	result, paintErr := paintShot(stepCtx, shot, outputFolder)

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
		state.shots[index].Error = textutil.TruncateRunes(paintErr.Error(), maxErrorRunes)
		event.Stage = fmt.Sprintf("第 %d / %d 镜绘制失败（%s）", index+1, total, shot.Scene)
		event.Summary = state.shots[index].Error
		event.Event = eventShotFailed
	} else {
		state.shots[index].Status = shotPainted
		state.shots[index].Handle = result.Handles[0]
		event.Stage = fmt.Sprintf("已完成第 %d / %d 镜（%s）", index+1, total, shot.Scene)
		event.Event = eventShotCompleted
	}
	state.nextShot = index + 1
	event.Shots = cloneShots(state.shots)
	applyEventLocked(state, event)
	commitVolatileLocked(state)
}

// advanceWriting 生成分镜文档并收尾。
func advanceWriting(taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stageWriting || state.canceled || state.paused {
		taskMu.Unlock()
		return
	}
	plan := state.plan
	shots := cloneShots(state.shots)
	outputFolder := state.outputFolder
	stopGuard := beginStepGuard(state, writingStepTimeout)
	taskMu.Unlock()
	defer stopGuard()

	doc, updated, err := writeStoryboardDoc(plan, shots, outputFolder)
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
	} else {
		// 全部分镜合成完成：把标题与句柄写入月华的「刚刚在做什么」运行时消息。
		activity.Set(fmt.Sprintf("刚演绎完《%s》，分镜与图片句柄：%s",
			plan.Title, strings.Join(handles, "、")))
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
	commitSnapshotLocked(state)
}

// failTask 把任务标记为失败并推送终止事件。
func failTask(taskID string, err error) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage == stageDone || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	reason := textutil.TruncateRunes(strings.TrimSpace(err.Error()), maxErrorRunes)
	state.stage = stageDone
	// 失败不是完成：保留失败时真实到达的进度，不再强行标 100%。
	applyEventLocked(state, ProgressEvent{
		Status:  statusFailed,
		Stage:   "执行失败",
		Summary: reason,
		Event:   eventFailed,
		Shots:   cloneShots(state.shots),
	})
	state.view.Error = reason
	commitSnapshotLocked(state)
	logger.Warn(Module, "演绎任务 %s 失败: %s", taskID, reason)
}

// isPausedOrCanceled 报告任务是否已被用户暂停或终止。
func isPausedOrCanceled(taskID string) bool {
	taskMu.Lock()
	defer taskMu.Unlock()
	state, ok := tasks[taskID]
	return ok && (state.paused || state.canceled)
}

// taskStepContext 单个执行步骤的超时与卡死兜底；调用方必须持有 taskMu。
//
// 只按「本步骤自己的超时」计时，不再用任务总时长裁剪：
// 大任务包含几十个长步骤，按剩余总时长裁剪会让正常步骤提前失败。
// 步骤超时后智能体应当返回；若它连上下文都不响应，兜底看门狗在
// step+stepStallGrace 之后确认「仍停在同一阶段且没有新状态」才判卡死。
func taskStepContext(ctx context.Context, state *taskState, step time.Duration) (context.Context, context.CancelFunc) {
	stopGuard := beginStepGuard(state, step)
	stepCtx, cancel := context.WithTimeout(ctx, step)
	return stepCtx, func() {
		stopGuard()
		cancel()
	}
}

// beginStepGuard 为当前原子步骤装上卡死兜底；调用方必须持有 taskMu。
//
// 返回的函数在步骤正常结束时调用，用于撤销兜底计时。
// 没有上下文的步骤（受理读正文、收尾写文档）同样必须装上它，
// 否则那两步卡住时没有任何机制能判定失败。
func beginStepGuard(state *taskState, limit time.Duration) func() {
	taskID := state.view.TaskID
	stage := state.stage
	guard := time.AfterFunc(limit+stepStallGrace, func() {
		failStalledStep(taskID, stage, limit)
	})
	return func() {
		guard.Stop()
	}
}

// failStalledStep 单步兜底：只有「超时后仍停在同一阶段且没有任何状态更新」才判卡死。
//
// 任何一次状态更新（含逐镜进度、过程文本）都会刷新 lastBeat，
// 因此步骤内部只要还在产出进度，就不会被这里误杀。
func failStalledStep(taskID, stage string, step time.Duration) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stage || state.paused || state.canceled || state.stage == stageDone {
		taskMu.Unlock()
		return
	}
	if time.Since(state.lastBeat) < step+stepStallGrace {
		// 步骤期间仍有状态更新：不判卡死，交给下一次调度继续推进。
		taskMu.Unlock()
		return
	}
	stageText := state.view.Stage
	taskMu.Unlock()
	reason := fmt.Errorf("步骤「%s」已超过 %s 没有任何状态更新，判定为执行卡死（自愈失败，请重新提交）",
		stageText, humanDuration(step+stepStallGrace))
	logger.Warn(Module, "演绎任务 %s 单步卡死: %v", taskID, reason)
	failTask(taskID, reason)
}
