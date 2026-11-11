package storyboard

// resume.go 任务现场的快照与还原：重启后从落盘状态继续推进。
//
// 能续跑的前提是「模型调用之外的状态都在盘上」：请求、故事正文、资料检索进度、
// 分镜计划、逐镜结果与下一个待绘制镜头。模型调用本身不保存，恢复后从当前阶段重新发起。

import (
	"encoding/json"
	"strings"
	"time"
)

// snapshotTaskLocked 把内存任务转成可落盘的完整现场；调用方必须持有 taskMu。
func snapshotTaskLocked(state *taskState) persistedTask {
	return persistedTask{
		View:         state.view,
		Request:      state.req,
		Stage:        state.stage,
		Story:        state.story,
		Thought:      state.thought,
		Research:     snapshotResearch(state.research),
		Plan:         snapshotPlan(state.plan),
		OutputFolder: state.outputFolder,
		Shots:        cloneShots(state.shots),
		NextShot:     state.nextShot,
		Paused:       state.paused,
		Canceled:     state.canceled,
	}
}

// decodePersistedTask 解析一条落盘任务；full=false 表示旧格式（只有任务视图）。
func decodePersistedTask(raw json.RawMessage) (persistedTask, TaskView, bool) {
	var record persistedTask
	if err := json.Unmarshal(raw, &record); err == nil && strings.TrimSpace(record.View.TaskID) != "" {
		return record, record.View, true
	}
	var legacy TaskView
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return persistedTask{}, legacy, false
	}
	return persistedTask{}, TaskView{}, false
}

// restoreTaskState 把落盘现场还原为可推进状态；nil 表示关键状态缺失，无法续跑。
func restoreTaskState(record persistedTask, now time.Time) *taskState {
	stage := record.Stage
	// 缺少必要现场时回退到更早阶段重做，而不是直接判死。
	switch stage {
	case stageQueued:
		if strings.TrimSpace(record.Request.Text) == "" && strings.TrimSpace(record.Request.Handle) == "" {
			return nil
		}
	case stageResearching, stagePlanning:
		if strings.TrimSpace(record.Story) == "" {
			stage = stageQueued
		}
	case stagePainting, stageWriting, stageReviewing:
		if len(record.Plan.Shots) == 0 {
			if strings.TrimSpace(record.Story) == "" {
				return nil
			}
			stage = stagePlanning
			record.Shots = nil
		}
	default:
		return nil
	}

	plan := restorePlan(record.Plan)
	shots := cloneShots(record.Shots)
	if len(shots) != len(plan.Shots) {
		shots = shotsFromPlan(plan, shotPlanned)
	}
	next := record.NextShot
	for next < len(shots) && shots[next].Status == shotPainted {
		next++
	}

	view := record.View
	view.Shots = cloneShots(shots)
	view.Status = stageStatus(stage)
	view.UpdatedAt = now.Format(time.RFC3339)
	view.Event = eventResumed
	view.Summary = "系统重启后已恢复该任务的执行现场"
	switch {
	case record.Paused:
		view.Status = statusPaused
		view.Stage = "已暂停（系统重启后保留）"
	case stage == stageReviewing:
		view.Stage = "分镜已生成，等待你的审查确认（系统重启后已恢复）"
	default:
		view.Stage = "已恢复，继续推进（" + emptyStageText(view.Stage) + "）"
	}

	state := &taskState{
		view:         view,
		req:          record.Request,
		stage:        stage,
		story:        record.Story,
		thought:      record.Thought,
		research:     restoreResearch(record.Research),
		plan:         plan,
		outputFolder: record.OutputFolder,
		shots:        shots,
		nextShot:     next,
		startedAt:    now,
		lastBeat:     now,
		budgetAt:     now.Add(runBudget),
		nextWarn:     now.Add(runWarnTimeout),
		paused:       record.Paused,
	}
	if record.Paused || stage == stageReviewing {
		state.restedAt = now
	}
	return state
}

// emptyStageText 阶段文案为空时给一个可读兜底。
func emptyStageText(stage string) string {
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return "原阶段"
	}
	return stage
}

// snapshotResearch 资料准备进度转落盘形式。
func snapshotResearch(research researchState) persistedResearch {
	return persistedResearch{
		Planned: research.planned,
		Queries: append([]string(nil), research.queries...),
		Index:   research.index,
		Notes:   append([]string(nil), research.notes...),
	}
}

// restoreResearch 还原资料准备进度。
func restoreResearch(stored persistedResearch) researchState {
	return researchState{
		planned: stored.Planned,
		queries: append([]string(nil), stored.Queries...),
		index:   stored.Index,
		notes:   append([]string(nil), stored.Notes...),
	}
}

// snapshotPlan 分镜计划转落盘形式。
func snapshotPlan(plan storyboardPlan) persistedPlan {
	out := persistedPlan{Title: plan.Title, Style: plan.Style}
	for index, shot := range plan.Shots {
		shotIndex := shot.Index
		if shotIndex <= 0 {
			shotIndex = index + 1
		}
		out.Shots = append(out.Shots, persistedShot{
			Index:      shotIndex,
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
		})
	}
	return out
}

// restorePlan 还原分镜计划。
func restorePlan(stored persistedPlan) storyboardPlan {
	out := storyboardPlan{Title: stored.Title, Style: stored.Style}
	for index, shot := range stored.Shots {
		shotIndex := shot.Index
		if shotIndex <= 0 {
			shotIndex = index + 1
		}
		out.Shots = append(out.Shots, plannedShot{
			Index:      shotIndex,
			Act:        shot.Act,
			Scene:      shot.Scene,
			Narration:  shot.Narration,
			Continuity: shot.Continuity,
			Prompt:     shot.Prompt,
			Ratio:      shot.Ratio,
		})
	}
	return out
}
