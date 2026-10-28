package debugsnap

// type.go 调试快照的类型定义。

import (
	"encoding/json"
	"time"
)

// Alert 触发一次调试快照的告警事件。
type Alert struct {
	Level   string    // 日志级别：WARN / ERROR / FATAL
	Module  string    // 模块名
	Sub     string    // 子模块名（可为空）
	Message string    // 告警正文（已按 maxMessageRunes 截断）
	At      time.Time // 触发时刻
}

// Meta 写入 meta.json 的快照元信息。
//
// 快照是「事后排查」用的：触发原因、时间、版本与三类数据的来源都必须自述清楚，
// 否则拿到目录也不知道它对应哪次告警、日志是全部还是最近一段。
type Meta struct {
	Kind            string   `json:"kind"`
	TriggeredAt     string   `json:"triggered_at"`
	Level           string   `json:"level"`
	Module          string   `json:"module"`
	Sub             string   `json:"sub,omitempty"`
	Message         string   `json:"message"`
	Version         string   `json:"version"`
	Developer       bool     `json:"developer"`
	RawLogsSource   string   `json:"raw_logs_source"`
	ReasoningSource string   `json:"reasoning_source"`
	AgentsSource    string   `json:"agents_source"`
	Files           []string `json:"files"`
	Note            string   `json:"note,omitempty"`
}

// AgentsContext 全部智能体上下文的导出载荷（agents-context.json）。
type AgentsContext struct {
	Kind        string       `json:"kind"`
	GeneratedAt string       `json:"generated_at"`
	Source      string       `json:"source"`
	Count       int          `json:"count"`
	Agents      []AgentEntry `json:"agents"`
}

// AgentEntry 单个智能体的导出结果。
type AgentEntry struct {
	ID      string          `json:"id"`
	Label   string          `json:"label,omitempty"`
	Group   string          `json:"group,omitempty"`
	Content json.RawMessage `json:"content,omitempty"` // 导出内容本身仍是 JSON
	Error   string          `json:"error,omitempty"`   // 该目标导出失败的原因（不影响其余目标）
}

// snapshotSources 三类快照数据的取数实现；测试可替换为假实现。
type snapshotSources struct {
	rawLogs   func() string
	reasoning func() (string, error)
	agents    func() (string, error)
}
