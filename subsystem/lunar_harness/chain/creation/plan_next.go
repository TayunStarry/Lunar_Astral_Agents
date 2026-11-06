package creation

// plan_next.go 自主选题入口：心情极性决定续做还是换新，事项由预设列表随机组合。

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"LunarSubsystem/LunarHarness/engine/activity"
)

// PlanNext 自主决定本轮创作事项：
//   - 心情偏正向：继续当前未完成事项，没有则恢复最近暂存的事项，再没有才新抽一项；
//   - 心情偏负面：把当前未完成事项转入暂存，再抽一个全新事项；
//   - 中性按偏正向处理，避免未完成事项被无谓丢弃。
func PlanNext(_ context.Context) (PlanItem, error) {
	planMu.Lock()
	defer planMu.Unlock()

	state := loadPlanState()
	polarity := polarityOf(moodSnapshotHook())

	if polarity != MoodNegative {
		if state.Active != nil {
			state.Active.Reason = polarityReason(polarity, true)
			savePlanState(state)
			return *state.Active, nil
		}
		if item, ok := popParked(&state); ok {
			item.Reason = polarityReason(polarity, true)
			state.Active = &item
			savePlanState(state)
			return item, nil
		}
		return activateNew(&state, polarityReason(polarity, false))
	}

	if state.Active != nil {
		parkItem(&state, *state.Active)
		state.Active = nil
	}
	return activateNew(&state, polarityReason(polarity, false))
}

// activateNew 从预设列表抽一个新事项并设为当前事项。
func activateNew(state *planState, reason string) (PlanItem, error) {
	rng := newRandHook()
	item, ok := drawItem(rng, state.Recent)
	if !ok {
		return PlanItem{}, errors.New("预设创作事项列表为空或全部不可用")
	}
	item.ID = newItemID(item.Kind)
	item.CreatedAt = nowUnix()
	item.Reason = reason
	state.Active = &item
	pushRecent(state, item.Kind)
	savePlanState(*state)
	return item, nil
}

// drawItem 随机组合事项：第一轮排除最近窗口内的类型，第二轮放宽以保证一定能选出。
func drawItem(rng *rand.Rand, recent []string) (PlanItem, bool) {
	qq := qqProbeHook()
	order := rng.Perm(len(itemSpecs))
	for pass := 0; pass < 2; pass++ {
		for _, index := range order {
			spec := itemSpecs[index]
			if pass == 0 && inRecent(spec.Kind, recent) {
				continue
			}
			if item, ok := spec.Build(rng, qq); ok {
				return item, true
			}
		}
	}
	return PlanItem{}, false
}

// inRecent 判断类型是否落在最近窗口内。
func inRecent(kind ItemKind, recent []string) bool {
	for _, item := range recent {
		if item == string(kind) {
			return true
		}
	}
	return false
}

// completePlan 把事项标记为已完成，清空当前事项。
func completePlan(item PlanItem) {
	planMu.Lock()
	defer planMu.Unlock()
	state := loadPlanState()
	if state.Active == nil || state.Active.ID != item.ID {
		return
	}
	state.Active = nil
	savePlanState(state)
}

// finishPlan 一轮创作收尾：标记事项完成，并立即更新「我刚刚在干的事情」。
func finishPlan(item PlanItem) {
	completePlan(item)
	activity.Set(item.Activity)
}

// withPlanInstruction 把自主选定的事项写在指令最前，替代旧的「对话者指定」来源。
func withPlanInstruction(instruction string, item PlanItem) string {
	var b strings.Builder
	b.WriteString("【本轮创作事项（系统自主选定，必须完成）】\n")
	fmt.Fprintf(&b, "- 类型：%s\n", kindLabel(item.Kind))
	fmt.Fprintf(&b, "- 事项：%s\n", item.Title)
	fmt.Fprintf(&b, "- 执行要求：%s\n", item.Task)
	fmt.Fprintf(&b, "- 选择依据：%s\n", item.Reason)
	b.WriteString("- 优先级说明：本轮事项替代「对话者主动输入或指定」的来源；下文若出现心情建议或创作参考，只影响风格与表达，不改变本轮事项的类型、主题与目标对象。\n\n")
	b.WriteString(instruction)
	return b.String()
}

// itemSeq 事项序号，保证同一纳秒内生成的事项 ID 也不重复。
var itemSeq atomic.Int64

// newItemID 生成事项 ID。
func newItemID(kind ItemKind) string {
	return fmt.Sprintf("%s-%d-%d", kind, time.Now().UnixNano(), itemSeq.Add(1))
}
