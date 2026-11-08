package creation

// plan_store.go 创作事项状态的持久化：未完成事项、暂存事项与最近类型窗口。

import (
	"encoding/json"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// loadPlanState 读取事项状态；文件缺失 / 损坏 / 版本不符时返回空状态。
func loadPlanState() planState {
	raw, ok := persistence.ReadFileBytes(planStateFile)
	if !ok {
		return planState{Version: planStateVersion}
	}
	var state planState
	if err := json.Unmarshal(raw, &state); err != nil {
		logger.Warn(Module, "[自主创作] 事项状态文件损坏，按空状态处理: %v", err)
		return planState{Version: planStateVersion}
	}
	if state.Version != planStateVersion {
		return planState{Version: planStateVersion}
	}
	if len(state.Recent) > recentWindow {
		state.Recent = state.Recent[len(state.Recent)-recentWindow:]
	}
	if len(state.Parked) > maxParked {
		state.Parked = state.Parked[len(state.Parked)-maxParked:]
	}
	if len(state.RecentTargets) > targetWindow {
		state.RecentTargets = state.RecentTargets[len(state.RecentTargets)-targetWindow:]
	}
	return state
}

// savePlanState 持久化事项状态；失败只告警，不影响本轮创作。
func savePlanState(state planState) {
	state.Version = planStateVersion
	state.UpdatedAt = nowUnix()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(Module, "[自主创作] 事项状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(planStateFile, raw); err != nil {
		logger.Warn(Module, "[自主创作] 事项状态写入失败: %v", err)
	}
}

// pushRecent 记录本次选中的类型，只保留最近的排除窗口。
func pushRecent(state *planState, kind ItemKind) {
	state.Recent = append(state.Recent, string(kind))
	if len(state.Recent) > recentWindow {
		state.Recent = state.Recent[len(state.Recent)-recentWindow:]
	}
}

// pushRecentTarget 记录本次选中的具体目标，只保留最近的排除窗口。
func pushRecentTarget(state *planState, target string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	state.RecentTargets = append(state.RecentTargets, target)
	if len(state.RecentTargets) > targetWindow {
		state.RecentTargets = state.RecentTargets[len(state.RecentTargets)-targetWindow:]
	}
}

// containsString 判断字符串切片是否包含指定值。
func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// parkItem 暂存未完成事项：同 ID 去重，超出上限时丢弃最早的一条。
func parkItem(state *planState, item PlanItem) {
	for i, parked := range state.Parked {
		if parked.ID == item.ID {
			state.Parked = append(state.Parked[:i], state.Parked[i+1:]...)
			break
		}
	}
	state.Parked = append(state.Parked, item)
	if len(state.Parked) > maxParked {
		state.Parked = state.Parked[len(state.Parked)-maxParked:]
	}
}

// popParked 取出最近暂存的未完成事项（后进先出）。
func popParked(state *planState) (PlanItem, bool) {
	if len(state.Parked) == 0 {
		return PlanItem{}, false
	}
	last := len(state.Parked) - 1
	item := state.Parked[last]
	state.Parked = state.Parked[:last]
	return item, true
}
