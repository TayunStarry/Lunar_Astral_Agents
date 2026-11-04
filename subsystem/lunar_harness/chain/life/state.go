package life

import (
	"encoding/json"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/persistence"
)

// RequestEvent 登记一次强制生活事件：下一 Tick 跳过权重选择，直接执行指定事件。
// instruction 是对话者/用户传入的自然语言要求，会随环境交给事件智能体。
func RequestEvent(eventID, instruction string) bool {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	if eventByIDLocked(eventID) == nil {
		logger.Warn(Module, "[生活] 请求的强制事件 %s 未注册", eventID)
		return false
	}
	state.PendingEvent = eventID
	state.PendingInstruction = strings.TrimSpace(instruction)
	saveStateLocked()
	logger.Info(Module, "[生活] 已登记强制事件 %s，下一 Tick 跳过权重选择", eventID)
	return true
}

// HasPendingEvent 是否有等待执行的强制事件。
func HasPendingEvent() bool {
	mu.Lock()
	defer mu.Unlock()
	return strings.TrimSpace(state.PendingEvent) != ""
}

// Init 载入生活状态并注册内置事件；可重复调用。
func Init() {
	mu.Lock()
	defer mu.Unlock()
	loadStateLocked()
	ensureDateLocked()
	registerBuiltinLocked()
	logger.Info(Module, "[生活] 初始化完成，今日额度=%v，上次记录=%s", state.Used, state.LastRecord)
	if state.Active != nil {
		logger.Info(Module, "[生活] 发现未完成事件 %s，将在空闲 Tick 续跑", state.Active.Event)
	}
}

// HasActiveRun 是否有未完成事件。
func HasActiveRun() bool {
	mu.Lock()
	defer mu.Unlock()
	return state.Active != nil
}

// Snapshot 返回只读快照。
func Snapshot() LifeSnapshot {
	mu.Lock()
	defer mu.Unlock()
	out := LifeSnapshot{LastRecord: state.LastRecord, PlanSummary: state.PlanSummary}
	if state.Active != nil {
		out.ActiveEvent = state.Active.Event
	}
	return out
}

// IdleThreshold 返回生活冷却阈值。
func IdleThreshold() int {
	return idleThreshold
}

func loadStateLocked() {
	raw, ok := persistence.ReadFileBytes(stateFile)
	if !ok {
		state = State{Version: 1, Used: map[string]int{}}
		return
	}
	var saved State
	if err := json.Unmarshal(raw, &saved); err != nil {
		logger.Warn(Module, "[生活] 状态文件损坏，使用默认状态: %v", err)
		state = State{Version: 1, Used: map[string]int{}}
		return
	}
	if saved.Version == 0 {
		saved.Version = 1
	}
	if saved.Used == nil {
		saved.Used = map[string]int{}
	}
	state = saved
}

func saveStateLocked() {
	state.Version = 1
	state.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		logger.Warn(Module, "[生活] 状态序列化失败: %v", err)
		return
	}
	if err := persistence.WriteFileAtomic(stateFile, raw); err != nil {
		logger.Warn(Module, "[生活] 状态写入失败: %v", err)
	}
}

// ensureDateLocked 跨自然日清零事件额度；生活记录与计划摘要保留。
func ensureDateLocked() {
	today := time.Now().Format("2006-01-02")
	if state.Date == today {
		return
	}
	state.Date = today
	state.Used = map[string]int{}
	saveStateLocked()
	logger.Info(Module, "[生活] 跨日，事件额度已清零")
}

func usedLocked(event string) int {
	return state.Used[event]
}

func recordLocked(record string) {
	state.LastRecord = record
}
