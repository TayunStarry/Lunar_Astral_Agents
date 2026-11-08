package storyboard

// persist.go 任务视图落盘与重启恢复。
//
// 演绎任务的绘制现场（图片句柄、外部会话）在进程重启后不可恢复，
// 因此恢复策略是：把上次未完成的任务标记为中断并推送原因，不做续跑。

import (
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// persistTasksLocked 把当前任务视图写入磁盘；调用方必须持有 taskMu。
func persistTasksLocked() {
	views := make([]TaskView, 0, len(taskOrder))
	for _, id := range taskOrder {
		if state, ok := tasks[id]; ok {
			views = append(views, state.view)
		}
	}
	raw, err := json.MarshalIndent(persistedTasks{Version: 1, Tasks: views}, "", "  ")
	if err != nil {
		logger.Warn(Module, "任务视图序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(storyboardTasksFile, raw); err != nil {
		logger.Warn(Module, "任务视图落盘失败: %v", err)
	}
}

// RecoverInterrupted 读取上次进程遗留的任务视图：
// 未结束的任务一律标记为失败并推送中断原因，已结束的任务原样恢复供查询。
func RecoverInterrupted() {
	raw, ok := persistence.ReadFileBytes(storyboardTasksFile)
	if !ok {
		return
	}
	var stored persistedTasks
	if err := json.Unmarshal(raw, &stored); err != nil {
		logger.Warn(Module, "任务视图文件损坏，跳过中断恢复: %v", err)
		return
	}
	if len(stored.Tasks) == 0 {
		return
	}

	now := time.Now().Format(time.RFC3339)
	interrupted := make([]TaskView, 0, len(stored.Tasks))

	taskMu.Lock()
	for _, view := range stored.Tasks {
		view.TaskID = strings.TrimSpace(view.TaskID)
		if view.TaskID == "" {
			continue
		}
		if view.Status != statusCompleted && view.Status != statusFailed {
			view.Status = statusFailed
			view.Stage = "已中断"
			view.Progress = 100
			view.Summary = recoverReason
			view.Error = recoverReason
			view.Event = eventInterrupted
			view.UpdatedAt = now
			interrupted = append(interrupted, view)
		}
		if _, exists := tasks[view.TaskID]; !exists {
			tasks[view.TaskID] = &taskState{view: view, stage: stageDone}
			taskOrder = append(taskOrder, view.TaskID)
		}
	}
	trimTasksLocked()
	persistTasksLocked()
	taskMu.Unlock()

	for _, view := range interrupted {
		logger.Warn(Module, "演绎任务 %s 因系统重启中断", view.TaskID)
		emitTask(view)
	}
}
