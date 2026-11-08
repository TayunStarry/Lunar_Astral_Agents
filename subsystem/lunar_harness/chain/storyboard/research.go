package storyboard

// research.go 资料准备阶段：调用「分镜规划-资料」智能体生成检索意图，
// 每个 Tick 执行一条网络搜索。
//
// 本文件只做编排：检索意图由 agentbase 智能体产出，本链不构造模型请求、
// 不解析模型流。检索结果作为规划输入，同时以可读文本推送。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/agents/StoryboardResearch"
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// advanceResearching 推进资料准备：先规划检索意图，再逐条执行。
func advanceResearching(ctx context.Context, taskID string) {
	taskMu.Lock()
	state, ok := tasks[taskID]
	if !ok || state.stage != stageResearching || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	if !state.research.planned {
		story := state.story
		stepCtx, cancel := taskStepContext(ctx, state, stageResearchTimeout())
		state.cancel = cancel
		taskMu.Unlock()

		plan, err := StoryboardResearch.Plan(stepCtx, story)
		cancel()

		taskMu.Lock()
		state, ok = tasks[taskID]
		if !ok || state.stage != stageResearching || state.paused || state.canceled {
			taskMu.Unlock()
			return
		}
		state.cancel = nil
		if err != nil {
			// 资料准备失败不阻断剧情：记录原因后直接进入规划。
			logger.Warn(Module, "演绎任务 %s 生成检索意图失败: %v", taskID, err)
			state.research.planned = true
			applyEventLocked(state, ProgressEvent{
				Status:       statusResearching,
				Progress:     4,
				Event:        eventThinking,
				ThoughtDelta: fmt.Sprintf("【资料准备】未能生成检索意图（%s），直接进入分镜规划。\n", clipRunes(err.Error(), 120)),
			})
			commitVolatileLocked(state)
			return
		}
		queries := plan.Queries
		state.research.planned = true
		state.research.queries = queries
		state.research.index = 0
		text := ""
		if analysis := strings.TrimSpace(plan.Analysis); analysis != "" {
			text = "【资料准备】" + analysis + "\n"
		}
		if len(queries) == 0 {
			text += "【资料准备】剧情设定完整，本轮无需外部检索。\n"
		} else {
			text += fmt.Sprintf("【资料准备】计划检索 %d 条资料：\n", len(queries))
			for index, query := range queries {
				text += fmt.Sprintf("%d. %s\n", index+1, query)
			}
		}
		applyEventLocked(state, ProgressEvent{
			Status:       statusResearching,
			Stage:        "正在补全设定资料",
			Progress:     3,
			Event:        eventThinking,
			ThoughtDelta: text,
		})
		commitVolatileLocked(state)
		return
	}

	if state.research.index >= len(state.research.queries) {
		state.stage = stagePlanning
		applyEventLocked(state, ProgressEvent{
			Status:   statusPlanning,
			Stage:    "资料准备完成，开始推演故事并拆分分镜",
			Progress: 5,
			Event:    eventResearchCompleted,
		})
		commitLocked(state)
		return
	}

	query := state.research.queries[state.research.index]
	stepCtx, cancel := taskStepContext(ctx, state, searchTimeout+30*time.Second)
	state.cancel = cancel
	taskMu.Unlock()

	note, err := runResearchQuery(stepCtx, query)
	cancel()

	taskMu.Lock()
	state, ok = tasks[taskID]
	if !ok || state.stage != stageResearching || state.paused || state.canceled {
		taskMu.Unlock()
		return
	}
	state.cancel = nil
	if err != nil {
		note = "检索失败：" + err.Error()
	}
	note = clipRunes(strings.TrimSpace(note), maxResearchNoteRunes)
	state.research.notes = append(state.research.notes,
		fmt.Sprintf("【检索 %d｜%s】\n%s", state.research.index+1, query, note))
	state.research.index++
	applyEventLocked(state, ProgressEvent{
		Status:       statusResearching,
		Stage:        fmt.Sprintf("资料准备：已完成 %d / %d 条检索", state.research.index, len(state.research.queries)),
		Progress:     3,
		Event:        eventThinking,
		ThoughtDelta: fmt.Sprintf("【检索】%s\n%s\n\n", query, note),
	})
	commitVolatileLocked(state)
}

// runResearchQuery 调用装配层注入的搜索能力，返回可读摘要。
func runResearchQuery(ctx context.Context, query string) (string, error) {
	hook := WebSearchHook
	if hook == nil {
		return "未配置网络搜索能力，已跳过该条检索。", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	note, err := hook(query)
	if err != nil {
		return "", err
	}
	return note, nil
}
