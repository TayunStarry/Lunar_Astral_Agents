//go:build windows

package desktop

// export.go 族级导出与运行辅助：各成员最近一次执行结果、族级故障上报。
//
// 编排循环在 chain/desktop；成员包只把各自的最近执行结果登记到这里，
// 由族级 dump/repair 条目统一导出。

import (
	"sync"
	"time"
)

var (
	// phaseLogMu 保护各成员最近一次执行结果
	phaseLogMu sync.Mutex
	// phaseLog 成员显示名 → 最近一次执行结果（供 agentdump 展示）
	phaseLog = map[string]map[string]any{}
)

// RecordCall 记录一次成员调用的执行结果（err 为 nil 表示成功）。
func RecordCall(member string, err error) {
	phaseLogMu.Lock()
	defer phaseLogMu.Unlock()
	entry := map[string]any{"at": time.Now().Format(time.RFC3339), "ok": err == nil}
	if err != nil {
		entry["error"] = err.Error()
	}
	phaseLog[member] = entry
}

// SnapshotCalls 返回各成员最近一次执行结果的副本。
func SnapshotCalls() map[string]any {
	phaseLogMu.Lock()
	defer phaseLogMu.Unlock()
	out := make(map[string]any, len(phaseLog))
	for member, entry := range phaseLog {
		copied := make(map[string]any, len(entry))
		for key, value := range entry {
			copied[key] = value
		}
		out[member] = copied
	}
	return out
}

// ResetCalls 清空各成员最近一次执行结果。
func ResetCalls() {
	phaseLogMu.Lock()
	phaseLog = map[string]map[string]any{}
	phaseLogMu.Unlock()
}
