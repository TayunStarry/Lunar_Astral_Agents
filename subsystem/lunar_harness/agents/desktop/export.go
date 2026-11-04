//go:build windows

package desktop

// export.go 桌控链使用的公开阶段入口。
//
// 编排循环已上移到 chain/desktop；这里只把各模型阶段与确定性辅助函数导出，
// 并记录最近一次阶段结果供 agentdump 展示。桌控链不内嵌模型调用。

import (
	"context"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// EditorTask 编纂者输出的结构化任务（等价于内部 editorTask）。
type EditorTask = editorTask

// PlanDecision 规划者的决策输出（等价于内部 planDecision）。
type PlanDecision = planDecision

// AutoMaxLoops 执行循环上限（配合「连续无变化即停滞」检测）。
const AutoMaxLoops = autoMaxLoops

var (
	// phaseLogMu 保护各阶段最近一次执行结果
	phaseLogMu sync.Mutex
	// phaseLog 最近一次各阶段执行结果（供 agentdump 展示）
	phaseLog = map[string]map[string]any{}
)

// recordPhase 记录一个阶段最近一次的执行结果。
func recordPhase(role string, err error) {
	phaseLogMu.Lock()
	defer phaseLogMu.Unlock()
	entry := map[string]any{"at": time.Now().Format(time.RFC3339), "ok": err == nil}
	if err != nil {
		entry["error"] = err.Error()
	}
	phaseLog[role] = entry
}

// snapshotPhases 返回各阶段最近一次执行结果的副本。
func snapshotPhases() map[string]any {
	phaseLogMu.Lock()
	defer phaseLogMu.Unlock()
	out := make(map[string]any, len(phaseLog))
	for role, entry := range phaseLog {
		copied := make(map[string]any, len(entry))
		for key, value := range entry {
			copied[key] = value
		}
		out[role] = copied
	}
	return out
}

// resetPhases 清空各阶段最近一次执行结果。
func resetPhases() {
	phaseLogMu.Lock()
	phaseLog = map[string]map[string]any{}
	phaseLogMu.Unlock()
}

// EditorPhase 提示词编纂者：将用户原始指令解析为结构化任务 JSON。
func EditorPhase(ctx context.Context, instruction string) (EditorTask, error) {
	out, err := editorPhase(ctx, instruction)
	recordPhase(autoRoleEditor, err)
	return out, err
}

// LauncherPhase 软件启动者：查找/打开目标应用并回填交接记录。
func LauncherPhase(ctx context.Context, plan EditorTask, handoff *HandoffRecord) error {
	err := launcherPhase(ctx, plan, handoff)
	recordPhase(autoRoleLauncher, err)
	return err
}

// UIAReaderPhase UIA 理解者：读当前应用的 UI 树并返回摘要与元素。
func UIAReaderPhase(ctx context.Context, plan EditorTask) (string, []string) {
	summary, elements := uiaReaderPhase(ctx, plan)
	recordPhase(autoRoleUIA, nil)
	return summary, elements
}

// PlannerPhase 任务规划者：综合 UI 情报与窗口状态做出下一步决策。
func PlannerPhase(ctx context.Context, plan EditorTask, handoff HandoffRecord, treeSummary string, elements []string) PlanDecision {
	decision := plannerPhase(ctx, plan, handoff, treeSummary, elements)
	recordPhase(autoRolePlanner, nil)
	return decision
}

// OperationPhase 单一操作者：按规划执行一次键盘 / 鼠标 / UIA 操作。
func OperationPhase(ctx context.Context, decision PlanDecision, handoff HandoffRecord) ActionAttempt {
	attempt := operationPhase(ctx, decision, handoff)
	recordPhase(autoRolePlanner+"·操作", nil)
	return attempt
}

// EnsureTargetForeground 每轮开始确认目标应用在前台。
func EnsureTargetForeground(ctx context.Context, target string) error {
	return ensureTargetForeground(ctx, target)
}

// ForegroundProcess 返回当前前台窗口进程名。
func ForegroundProcess() string {
	return dtForegroundProcess()
}

// AdvanceHandoff 由编排层确定性推进交接记录。
func AdvanceHandoff(handoff HandoffRecord, attempt ActionAttempt) HandoffRecord {
	return advanceHandoff(handoff, attempt)
}

// AppendAttempt 追加一次尝试并只保留最近若干条。
func AppendAttempt(list []ActionAttempt, attempt ActionAttempt) []ActionAttempt {
	return appendAttempt(list, attempt)
}

// ReportFailure 把编排失败上报给智能修复（人工中断不算故障）。
func ReportFailure(err error) {
	agentbase.ReportRun("desktop", err)
}
