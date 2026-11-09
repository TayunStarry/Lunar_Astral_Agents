package creation

// jobs.go 资源库提交的创作任务：受理即入队，由 Tick 调度器按优先级逐个推进，
// 保证同一时刻只有一条链在跑；状态可查询并推送。

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// SubmitTask 受理一个创作任务并立即返回任务标识；真正的规划与执行交给 Tick 调度器。
func SubmitTask(req SubmitRequest) (TaskView, error) {
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "assigned"
	}
	if mode != "assigned" && mode != "random" {
		return TaskView{}, fmt.Errorf("mode 只支持 assigned 或 random")
	}
	description := strings.TrimSpace(req.Description)
	if mode == "assigned" && description == "" {
		return TaskView{}, fmt.Errorf("assigned 模式需要提供 description")
	}

	now := time.Now().Format(time.RFC3339)
	seq := atomic.AddUint64(&taskSeq, 1)
	view := TaskView{
		TaskID:    fmt.Sprintf("creation-%d-%d", time.Now().UnixNano(), seq),
		Mode:      mode,
		Status:    statusQueued,
		Stage:     "已受理，等待创作队列",
		Progress:  0,
		CreatedAt: now,
		UpdatedAt: now,
	}
	state := &taskState{
		view:        view,
		instruction: description,
		handles:     cleanHandles(req.Handles),
	}

	taskMu.Lock()
	tasks[view.TaskID] = state
	taskOrder = append(taskOrder, view.TaskID)
	trimTasksLocked()
	taskMu.Unlock()

	emitTask(view)
	return view, nil
}

// HasWork 报告是否存在等待推进的创作任务。
func HasWork() bool {
	taskMu.Lock()
	defer taskMu.Unlock()
	for _, id := range taskOrder {
		if state, ok := tasks[id]; ok && state.view.Status == statusQueued {
			return true
		}
	}
	return false
}

// Step 推进最早一个等待中的创作任务；每次 Tick 最多推进一个任务。
func Step(ctx context.Context) error {
	taskMu.Lock()
	var (
		taskID      string
		mode        string
		instruction string
		handles     []string
	)
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if !ok || state.view.Status != statusQueued {
			continue
		}
		taskID = id
		mode = state.view.Mode
		instruction = state.instruction
		handles = append([]string(nil), state.handles...)
		// 先落执行态再放锁：即使本轮耗时较长，也不会被下一个 Tick 重复领取。
		state.view.Status = statusPlanning
		state.view.Stage = "开始规划"
		state.view.UpdatedAt = time.Now().Format(time.RFC3339)
		break
	}
	taskMu.Unlock()
	if taskID == "" {
		return nil
	}
	runTask(ctx, taskID, mode, instruction, handles)
	return nil
}

// TaskStatus 查询单个任务。
func TaskStatus(taskID string) (TaskView, bool) {
	taskMu.Lock()
	defer taskMu.Unlock()
	state, ok := tasks[strings.TrimSpace(taskID)]
	if !ok {
		return TaskView{}, false
	}
	return state.view, true
}

// TaskList 按创建顺序倒序返回最近任务。
func TaskList(limit int) []TaskView {
	if limit <= 0 || limit > maxTaskList {
		limit = maxTaskList
	}
	taskMu.Lock()
	defer taskMu.Unlock()
	out := make([]TaskView, 0, limit)
	for i := len(taskOrder) - 1; i >= 0 && len(out) < limit; i-- {
		if state, ok := tasks[taskOrder[i]]; ok {
			out = append(out, state.view)
		}
	}
	return out
}

// runTask 执行一个任务；进度通过 TaskEventHook 推送。
func runTask(ctx context.Context, taskID, mode, description string, handles []string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Warn(Module, "[资源库] 创作任务 %s 异常中断: %v", taskID, recovered)
			updateTask(taskID, ProgressEvent{
				Status:  statusFailed,
				Stage:   "执行异常中断",
				Summary: fmt.Sprintf("%v", recovered),
			})
		}
	}()
	observe := func(event ProgressEvent) {
		updateTask(taskID, event)
	}
	var (
		result TaskResult
		err    error
	)
	if mode == "random" {
		result, err = RunFreeWithProgress(ctx, observe)
	} else {
		result, err = RunAssignedWithProgress(ctx, description, handles, observe)
	}
	if err != nil {
		logger.Warn(Module, "[资源库] 创作任务 %s 失败: %v", taskID, err)
		updateTask(taskID, ProgressEvent{
			Status:   statusFailed,
			Stage:    "执行失败",
			Progress: 100,
			Summary:  err.Error(),
		})
		return
	}
	updateTask(taskID, ProgressEvent{
		Status:   statusCompleted,
		Stage:    "本轮创作已完成",
		Progress: 100,
		Summary:  result.Summary,
		Handles:  result.Handles,
	})
}

// updateTask 合并进度事件并推送。
func updateTask(taskID string, event ProgressEvent) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok {
		taskMu.Unlock()
		return
	}
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
	if len(event.Handles) > 0 {
		state.view.Handles = append([]string(nil), event.Handles...)
	}
	if event.Status == statusFailed && event.Summary != "" {
		state.view.Error = event.Summary
	}
	state.view.UpdatedAt = time.Now().Format(time.RFC3339)
	view := state.view
	taskMu.Unlock()
	emitTask(view)
}

// emitTask 调用装配层注入的推送回调。
func emitTask(view TaskView) {
	if hook := TaskEventHook; hook != nil {
		hook(view)
	}
}

// trimTasksLocked 保留最近的 maxTasks 个任务，优先淘汰已结束的旧任务；调用方需持有 taskMu。
func trimTasksLocked() {
	if len(taskOrder) <= maxTasks {
		return
	}
	excess := len(taskOrder) - maxTasks
	kept := make([]string, 0, len(taskOrder))
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if excess > 0 && ok && (state.view.Status == statusCompleted || state.view.Status == statusFailed) {
			delete(tasks, id)
			excess--
			continue
		}
		kept = append(kept, id)
	}
	taskOrder = kept
}
