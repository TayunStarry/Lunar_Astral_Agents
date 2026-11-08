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
	now := time.Now().Format(time.RFC3339)
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
		view:     view,
		req:      req,
		stage:    stageQueued,
		deadline: time.Now().Add(runTimeout),
	}

	taskMu.Lock()
	tasks[view.TaskID] = state
	taskOrder = append(taskOrder, view.TaskID)
	trimTasksLocked()
	persistTasksLocked()
	taskMu.Unlock()

	emitTask(view)
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
	return cloneTaskView(state), true
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
		state.view.Thought = ""
		state.thoughtReset = true
	}
	if event.ThoughtDelta != "" {
		state.thought += event.ThoughtDelta
		state.view.Thought = state.thought
		state.delta = event.ThoughtDelta
	}
	state.view.Event = event.Event
	state.view.ShotIndex = event.ShotIndex
	state.view.UpdatedAt = time.Now().Format(time.RFC3339)
}

// takeViewLocked 克隆视图并把待推送的过程文本增量挂上；调用方必须持有锁。
//
// 增量只存在于推送快照中，不写入任务视图本身：否则轮询查询会把同一段
// 文本重复返回，前端就会重复追加。
func takeViewLocked(state *taskState) TaskView {
	view := cloneTaskView(state)
	if state.delta != "" {
		view.Delta = state.delta
		state.delta = ""
	}
	if state.thoughtReset {
		view.ThoughtReset = true
		state.thoughtReset = false
	}
	return view
}

// commitLocked 落盘并解锁，随后推送当前视图；调用方必须持有锁。
func commitLocked(state *taskState) {
	view := takeViewLocked(state)
	persistTasksLocked()
	taskMu.Unlock()
	emitTask(view)
}

// commitVolatileLocked 解锁并推送当前视图，但不落盘。
// 用于高频的逐镜与逐段过程事件：落盘只需覆盖阶段切换与终态，
// 避免每次增量都重写整个任务文件。
func commitVolatileLocked(state *taskState) {
	view := takeViewLocked(state)
	taskMu.Unlock()
	emitTask(view)
}

// emitTask 调用装配层注入的推送回调。
func emitTask(view TaskView) {
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
			delete(tasks, id)
			excess--
			continue
		}
		kept = append(kept, id)
	}
	taskOrder = kept
}
