package storyboard

// jobs.go 演绎任务表：受理即返回任务标识，由 Tick 调度器按原子步骤推进。

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// SubmitTask 受理一个演绎任务并立即返回任务标识。
//
// 这里只入队并落盘，不再启动后台 goroutine：后续由 storyboardSystem 在
// 每个 Tick 调用 Advance 推进一个原子步骤，保证同一时刻只有一条链在运行。
func SubmitTask(req SubmitRequest) (TaskView, error) {
	if strings.TrimSpace(req.Text) == "" && strings.TrimSpace(req.Handle) == "" {
		return TaskView{}, fmt.Errorf("请提供故事正文 text 或资源句柄 handle")
	}
	if text := strings.TrimSpace(req.Text); text != "" {
		if err := validateStoryLength(text); err != nil {
			return TaskView{}, err
		}
	}
	created := time.Now()
	now := created.Format(time.RFC3339)
	seq := atomic.AddUint64(&taskSeq, 1)
	view := TaskView{
		TaskID:    fmt.Sprintf("storyboard-%d-%d", time.Now().UnixNano(), seq),
		Status:    statusQueued,
		Stage:     "已受理，等待演绎队列",
		Progress:  0,
		Event:     eventAccepted,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state := &taskState{
		view:      view,
		req:       req,
		stage:     stageQueued,
		startedAt: created,
		lastBeat:  created,
		budgetAt:  created.Add(runBudget),
		nextWarn:  created.Add(runWarnTimeout),
	}

	taskMu.Lock()
	tasks[view.TaskID] = state
	taskOrder = append(taskOrder, view.TaskID)
	trimTasksLocked()
	persistTasksLocked()
	// 受理即推送一份完整快照：前端据此建立任务模型，后续都是增量。
	pushView, _ := buildPushLocked(state, true)
	taskMu.Unlock()

	emitTask(pushView)
	// 回执与推送共用同一个快照序号：客户端先收到推送、再收到回执，不会重复建模型。
	view.Seq = pushView.Seq
	view.Epoch = pushView.Epoch
	view.V = pushVersion
	return view, nil
}

// HasWork 报告是否存在可推进的演绎任务。
//
// 审查态与暂停态不占用 Tick：用户等待期间其他链照常运行。
func HasWork() bool {
	taskMu.Lock()
	defer taskMu.Unlock()
	return oldestActionableLocked() != nil
}

// TaskStatus 查询单个任务。
func TaskStatus(taskID string) (TaskView, bool) {
	taskMu.Lock()
	defer taskMu.Unlock()
	state, ok := tasks[strings.TrimSpace(taskID)]
	if !ok {
		return TaskView{}, false
	}
	// 查询返回的是全量视图：先消费掉待推送的增量，再把过程文本游标对齐，
	// 避免随后到达的增量推送把同一段文本重复补发。
	consumePendingLocked(state)
	view := cloneTaskView(state)
	view.V = pushVersion
	view.ShotCount = len(view.Shots)
	view.Thought = state.thought
	state.sentThought = len(state.thought)
	view.Seq = atomic.AddUint64(&viewSeq, 1)
	view.Epoch = taskEpoch
	state.view.Seq = view.Seq
	return view, true
}

// consumePendingLocked 丢弃尚未发出的增量；调用方必须持有 taskMu。
func consumePendingLocked(state *taskState) {
	if state.pushTimer != nil {
		state.pushTimer.Stop()
		state.pushTimer = nil
	}
	state.pushPending = false
	state.dirtyAll = false
	state.dirtyShots = nil
}

// TaskList 按创建顺序倒序返回最近任务。
func TaskList(limit int) []TaskView {
	if limit <= 0 || limit > maxTaskList {
		limit = maxTaskList
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	out := make([]TaskView, 0, limit)
	for index := len(taskOrder) - 1; index >= 0 && len(out) < limit; index-- {
		if state, ok := tasks[taskOrder[index]]; ok {
			out = append(out, cloneTaskView(state))
		}
	}
	return out
}

// oldestActionableLocked 返回最早创建、当前可推进的任务；调用方必须持有锁。
//
// 只有 queued / researching / planning / revising / painting / writing 可推进；
// reviewing 与 paused 直接跳过，让后续任务和其他链继续运行。
func oldestActionableLocked() *taskState {
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if !ok {
			continue
		}
		if state.stage == stageDone || state.paused || state.canceled {
			continue
		}
		switch state.stage {
		case stageQueued, stageResearching, stagePlanning, stageRevising, stagePainting, stageWriting:
			return state
		}
	}
	return nil
}

// applyEventLocked 把增量事件合并进任务视图；调用方必须持有锁。
func applyEventLocked(state *taskState, event ProgressEvent) {
	if event.Status != "" {
		state.view.Status = event.Status
	}
	if event.Stage != "" {
		state.view.Stage = event.Stage
	}
	if event.Progress > 0 && event.Progress > state.view.Progress {
		progress := event.Progress
		if progress > 100 {
			progress = 100
		}
		state.view.Progress = progress
	}
	if strings.TrimSpace(event.Summary) != "" {
		state.view.Summary = event.Summary
	}
	if event.Title != "" {
		state.view.Title = event.Title
	}
	if event.DocPath != "" {
		state.view.DocPath = event.DocPath
	}
	if event.DocHandle != "" {
		state.view.DocHandle = event.DocHandle
	}
	if len(event.Shots) > 0 {
		// 先记录差异再覆盖：推送只带变化的镜头，前端按序合并。
		markChangedShotsLocked(state, event.Shots)
		state.view.Shots = cloneShots(event.Shots)
	}
	if len(event.Handles) > 0 {
		state.view.Handles = append([]string(nil), event.Handles...)
	}
	if event.Status == statusFailed && event.Summary != "" {
		state.view.Error = event.Summary
	}
	if event.ThoughtReset {
		state.thought = ""
		state.thoughtReset = true
	}
	if event.ThoughtDelta != "" {
		state.thought = appendThoughtBounded(state.thought, event.ThoughtDelta)
	}
	state.view.Event = event.Event
	state.view.ShotIndex = event.ShotIndex
	state.view.UpdatedAt = time.Now().Format(time.RFC3339)
	// 任何一次状态合并都算「有进展」：卡死判定只认这个时间点。
	state.lastBeat = time.Now()
}

// takeViewLocked 克隆视图作为控制回执；调用方必须持有锁。
//
// 回执是完整快照（前端据此整体对齐），因此这里不需要处理增量。
func takeViewLocked(state *taskState) TaskView {
	view := cloneTaskView(state)
	view.V = pushVersion
	view.ShotCount = len(view.Shots)
	view.Seq = atomic.AddUint64(&viewSeq, 1)
	view.Epoch = taskEpoch
	state.view.Seq = view.Seq
	// 回执不含累计过程文本：文本只走增量推送，避免大对象来回搬运。
	view.Thought = ""
	view.Delta = ""
	view.ThoughtReset = false
	state.sentThought = len(state.thought)
	return view
}

// commitLocked 落盘并立即推送一次增量快照；调用方必须持有锁。
func commitLocked(state *taskState) {
	persistTasksLocked()
	view, ok := buildPushLocked(state, false)
	taskMu.Unlock()
	if ok {
		emitTask(view)
	}
}

// commitSnapshotLocked 落盘并立即推送一份完整快照；调用方必须持有锁。
// 用于提交 / 审查 / 终态这类低频且镜头表整体变化的节点。
func commitSnapshotLocked(state *taskState) {
	persistTasksLocked()
	view, ok := buildPushLocked(state, true)
	taskMu.Unlock()
	if ok {
		emitTask(view)
	}
}

// commitVolatileLocked 只安排合并推送，不落盘；调用方必须持有锁。
// 用于高频的逐镜与逐段过程事件：落盘只需覆盖阶段切换与终态，
// 避免每次增量都重写整个任务文件；推送则由 emit.go 合并限流。
func commitVolatileLocked(state *taskState) {
	schedulePushLocked(state)
	taskMu.Unlock()
}

// emitTask 调用装配层注入的推送回调。
func emitTask(view TaskView) {
	// 重启恢复的视图来自旧进程的落盘文件，统一改盖当前 epoch。
	view.Epoch = taskEpoch
	if view.V == 0 {
		view.V = pushVersion
	}
	// 全量快照一定带得出镜头总数：前端据此裁剪本地多出的镜头。
	if view.ShotCount == 0 && len(view.Shots) > 0 {
		view.ShotCount = len(view.Shots)
	}
	if hook := TaskEventHook; hook != nil {
		hook(view)
	}
}

// trimTasksLocked 保留最近的 maxTasks 个任务，优先淘汰已结束的旧任务。
func trimTasksLocked() {
	if len(taskOrder) <= maxTasks {
		return
	}
	excess := len(taskOrder) - maxTasks
	kept := make([]string, 0, len(taskOrder))
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if excess > 0 && ok && (state.view.Status == statusCompleted ||
			state.view.Status == statusFailed || state.view.Status == statusCanceled) {
			if state.pushTimer != nil {
				state.pushTimer.Stop()
				state.pushTimer = nil
			}
			delete(tasks, id)
			excess--
			continue
		}
		kept = append(kept, id)
	}
	taskOrder = kept
}
