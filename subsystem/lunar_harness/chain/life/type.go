package life

// type.go 生活体系的类型定义。

import (
	"context"
	"encoding/json"
	"time"
)

// State 生活体系持久化状态。
type State struct {
	Version     int            `json:"version"`
	Date        string         `json:"date"`
	Used        map[string]int `json:"used"`
	Active      *ActiveRun     `json:"active,omitempty"`
	LastRecord  string         `json:"last_record"`
	PlanSummary string         `json:"plan_summary"`
	// PendingEvent 工具触发的强制事件：下一 Tick 跳过权重选择直接执行
	PendingEvent string `json:"pending_event,omitempty"`
	// PendingInstruction 强制事件的自然语言要求，用于引导智能体流程
	PendingInstruction string `json:"pending_instruction,omitempty"`
	UpdatedAt          int64  `json:"updated_at"`
}

// ActiveRun 未完成的事件运行。
type ActiveRun struct {
	Event     string          `json:"event"`
	Step      int             `json:"step"`
	Data      json.RawMessage `json:"data,omitempty"`
	StartedAt int64           `json:"started_at"`
}

// LifeSnapshot 供对话运行时消息读取的只读快照。
type LifeSnapshot struct {
	LastRecord  string
	PlanSummary string
	ActiveEvent string
}

// Env 一次事件步骤可用的环境。
type Env struct {
	Now          time.Time
	MoodLabel    string
	LastRecord   string
	LastUserText string
	// Instruction 工具触发的自然语言要求；普通权重事件为空。
	Instruction string
}

// Event 可注册的生活事件。
type Event interface {
	ID() string
	Weight() int
	DailyQuota() int
	NewRun() EventRun
}

// EventRun 一次事件运行；Next 每次只做一个原子步骤。
type EventRun interface {
	Start(ctx context.Context, env Env) error
	Next(ctx context.Context, env Env) (EventStep, error)
	Snapshot() json.RawMessage
	Restore(json.RawMessage) error
}

// NoWorkRun 可选接口：事件启动后即可判定本次没有实际动作（如没有待合并组）。
// 调度层据此回滚刚扣除的当日额度，保证「空跑不消耗额度」。
type NoWorkRun interface {
	NoWork() bool
}

// EventStep 一个原子步骤的结果。
type EventStep struct {
	Done   bool
	Record string
	Detail string
}

// ==== 记忆整理 ====

type memoryItem struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Role      string `json:"role,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	// Pending 待处理（待合并）标记：写入时命中高度相似条目，等待整理合并
	Pending bool `json:"pending,omitempty"`
	// PendingPartner 待合并组中的高度相似既有条目 ID
	PendingPartner string `json:"pending_partner,omitempty"`
}

// memoryBatch 一个待合并组：组内条目语义相近，交给主推理模型合并成一条或多条新条目。
type memoryBatch struct {
	Items []memoryItem `json:"items"`
}

type memoryPlan struct {
	// Version 计划格式版本；旧版按时间分批的计划不再续跑，避免绕过相似度门槛整批改写
	Version int           `json:"version,omitempty"`
	ID      string        `json:"id"`
	Batches []memoryBatch `json:"batches"`
}

type memoryRunData struct {
	PlanFile string `json:"plan_file"`
	Index    int    `json:"index"`
	Total    int    `json:"total"`
	// Reason 不整理时的原因（触发条件不满足）；有值且 Total=0 时直接视为完成
	Reason string `json:"reason,omitempty"`
}

// memoryJournal 单批记忆整理的意图日志。
// Status: prepared -> replaced -> done；失败保留中间状态供下次恢复重放。
// 恢复重放通过引擎 ReplaceTexts 的「完全同文复用 + 缺失按已删」保证幂等。
type memoryJournal struct {
	BatchID     string       `json:"batch_id"`
	Status      string       `json:"status"`
	OldEntries  []memoryItem `json:"old_entries"`
	NewTexts    []string     `json:"new_texts"`
	DeletedIDs  []string     `json:"deleted_ids,omitempty"`
	WrittenIDs  []string     `json:"written_ids,omitempty"`
	ReusedIDs   []string     `json:"reused_ids,omitempty"`
	BeforeCount int          `json:"before_count,omitempty"`
	AfterCount  int          `json:"after_count,omitempty"`
	Error       string       `json:"error,omitempty"`
	UpdatedAt   int64        `json:"updated_at"`
}
