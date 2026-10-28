package debugsnap

// variable.go 调试快照的常量与全局状态。

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// snapshotKind meta.json 的 kind 值（便于离线时识别文件用途）
	snapshotKind = "lunar-debug-snapshot"

	// 快照目录内的固定文件名
	rawLogsFile   = "raw-logs.log"
	reasoningFile = "reasoning.json"
	agentsFile    = "agents-context.json"
	metaFile      = "meta.json"

	// defaultCooldown 同一进程内两次快照的最小间隔：
	// 告警往往成串出现（一次失败引发多条 WARN/ERROR），60 秒一次足以留证据又不会灌满磁盘。
	defaultCooldown = time.Minute

	// maxMessageRunes 写入 meta.json 的告警正文上限（字符数），避免单条超长消息撑大元信息
	maxMessageRunes = 4000

	// 三类数据的来源说明（写进 meta.json，保证快照自述"数据从哪来、有多少"）
	rawLogsSourceNote   = "console.GetLogs(console.SourceRaw)：TUI 原始日志环形缓冲（最多 1000 行），JSON Lines"
	reasoningSourceNote = "console.BuildTranscriptJSON：核心智能实况（模型调用轮次环形缓冲，历史上限 30 轮）"
	agentsSourceNote    = "agentdump.Export：全部已注册智能体上下文（include_images=false）"
	snapshotNote        = "开发者模式告警快照：警告及以上日志自动触发，仅落本地磁盘，不打开任何前端窗口。"
)

var (
	// snapshotBusy 同一时刻只允许一个快照在写盘（并发告警不重复取数）
	snapshotBusy atomic.Bool

	// lastSnapshotAt 上次快照触发时刻（UnixNano；0 表示本进程尚未触发）
	lastSnapshotAt atomic.Int64

	// snapshotCooldown 限流间隔（生产为 defaultCooldown；测试可调小）
	snapshotCooldown = defaultCooldown

	// snapshotWG 跟踪后台快照任务，仅供测试等待完成（生产不读取）
	snapshotWG sync.WaitGroup

	// sources 三类数据的取数实现（测试可替换为假实现）
	sources = snapshotSources{
		rawLogs:   collectRawLogs,
		reasoning: collectReasoning,
		agents:    collectAgentsContext,
	}
)
