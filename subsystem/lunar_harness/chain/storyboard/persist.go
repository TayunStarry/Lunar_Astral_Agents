package storyboard

// persist.go 任务现场落盘与重启恢复。
//
// 落盘的不只是对外视图，还包括续跑所需的内部状态：请求、故事正文、资料检索进度、
// 分镜计划、逐镜结果与下一个待绘制镜头。重启后：
//   - 已完成 / 已失败：原样恢复供查询；
//   - 已取消：保持取消，不续跑；
//   - 暂停 / 审查：恢复现场并停在原状态等待用户操作；
//   - 其余未完成任务：从落盘阶段继续推进，绘制从下一个未完成镜头接上；
//   - 旧格式（只有任务视图）或关键状态缺失：按中断处理并推送原因。

import (
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// persistTasksLocked 把全部任务的完整现场写入磁盘；调用方必须持有 taskMu。
func persistTasksLocked() {
	records := make([]json.RawMessage, 0, len(taskOrder))
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if !ok {
			continue
		}
		raw, err := json.Marshal(snapshotTaskLocked(state))
		if err != nil {
			logger.Warn(Module, "任务现场序列化失败（%s）: %v", id, err)
			continue
		}
		records = append(records, raw)
	}
	raw, err := json.MarshalIndent(persistedTasks{Version: 2, Tasks: records}, "", "  ")
	if err != nil {
		logger.Warn(Module, "任务现场落盘序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(storyboardTasksFile, raw); err != nil {
		logger.Warn(Module, "任务现场落盘失败: %v", err)
	}
}

// RecoverInterrupted 读取上次进程遗留的任务：
// 能还原现场的继续推进，无法还原的标记中断并推送原因。
func RecoverInterrupted() {
	raw, ok := persistence.ReadFileBytes(storyboardTasksFile)
	if !ok {
		return
	}
	var stored persistedTasks
	if err := json.Unmarshal(raw, &stored); err != nil {
		logger.Warn(Module, "任务现场文件损坏，跳过恢复: %v", err)
		return
	}
	if len(stored.Tasks) == 0 {
		return
	}

	now := time.Now()
	interrupted := make([]TaskView, 0)
	resumed := make([]TaskView, 0)

	taskMu.Lock()
	for _, rawTask := range stored.Tasks {
		record, view, full := decodePersistedTask(rawTask)
		view.TaskID = strings.TrimSpace(view.TaskID)
		if view.TaskID == "" {
			continue
		}
		if _, exists := tasks[view.TaskID]; exists {
			continue
		}
		if view.Status == statusCompleted || view.Status == statusFailed {
			tasks[view.TaskID] = &taskState{view: view, stage: stageDone}
			taskOrder = append(taskOrder, view.TaskID)
			continue
		}
		if !full {
			// 旧格式只有视图，没有可续跑的现场。
			markInterruptedLocked(&view, now)
			tasks[view.TaskID] = &taskState{view: view, stage: stageDone}
			taskOrder = append(taskOrder, view.TaskID)
			interrupted = append(interrupted, view)
			continue
		}
		if record.Canceled {
			view.Status = statusCanceled
			view.Stage = "已取消"
			view.Progress = 100
			view.Summary = "任务已取消，不再续跑"
			view.Event = eventInterrupted
			view.UpdatedAt = now.Format(time.RFC3339)
			view.Shots = nil
			tasks[view.TaskID] = &taskState{view: view, stage: stageDone, canceled: true}
			taskOrder = append(taskOrder, view.TaskID)
			continue
		}
		state := restoreTaskState(record, now)
		if state == nil {
			markInterruptedLocked(&view, now)
			tasks[view.TaskID] = &taskState{view: view, stage: stageDone}
			taskOrder = append(taskOrder, view.TaskID)
			interrupted = append(interrupted, view)
			continue
		}
		// 现场先还原，但不立刻推进：开机初期模型服务、显存与前端仍在初始化，
		// 此时抢占资源会直接卡死界面。等宿主确认系统就绪后由 ActivateRecovered 放行。
		if stageActionable(state.stage) && !state.paused {
			state.held = true
		}
		tasks[view.TaskID] = state
		taskOrder = append(taskOrder, view.TaskID)
		pushView, _ := buildPushLocked(state, true)
		resumed = append(resumed, pushView)
	}
	trimTasksLocked()
	persistTasksLocked()
	taskMu.Unlock()

	for _, view := range interrupted {
		logger.Warn(Module, "演绎任务 %s 因系统重启中断", view.TaskID)
		emitTask(view)
	}
	for _, view := range resumed {
		logger.Info(Module, "演绎任务 %s 已恢复现场，等待系统初始化完成后继续推进: %s", view.TaskID, view.Stage)
		emitTask(view)
	}
}

// ActivateRecovered 放行重启恢复的演绎任务：由宿主在系统完成全部初始化后调用。
//
// 放行前这些任务只保留现场，不占用 Tick、不发起任何模型调用；放行时把运行计时
// 统一改从当前时刻起算，让等待初始化的时间不计入任务运行时长与卡死判定。
func ActivateRecovered() int {
	now := time.Now()
	taskMu.Lock()
	activated := make([]TaskView, 0)
	for _, id := range taskOrder {
		state, ok := tasks[id]
		if !ok || !state.held {
			continue
		}
		state.held = false
		// 放行前已被用户暂停或终止的任务只清标记，不重置计时也不推送。
		if !stageActionable(state.stage) || state.paused || state.canceled {
			continue
		}
		state.startedAt = now
		state.lastBeat = now
		state.budgetAt = now.Add(runBudget)
		state.nextWarn = now.Add(runWarnTimeout)
		if view, okPush := buildPushLocked(state, true); okPush {
			activated = append(activated, view)
		}
	}
	if len(activated) > 0 {
		persistTasksLocked()
	}
	taskMu.Unlock()

	for _, view := range activated {
		logger.Info(Module, "演绎任务 %s 已放行，继续推进: %s", view.TaskID, view.Stage)
		emitTask(view)
	}
	return len(activated)
}

// markInterruptedLocked 把无法续跑的任务标记为中断；调用方必须持有 taskMu。
func markInterruptedLocked(view *TaskView, now time.Time) {
	view.Status = statusFailed
	view.Stage = "已中断"
	view.Progress = 100
	view.Summary = recoverReason
	view.Error = recoverReason
	view.Event = eventInterrupted
	view.UpdatedAt = now.Format(time.RFC3339)
	view.Shots = nil
}
