package websearch

import (
	"io"
	"sync"
)

// DelegateRole 辩论角色
type DelegateRole string

const (
	// RoleOptimist 维新派：从网络信息出发，寻找积极信号和发展机遇
	RoleOptimist DelegateRole = "optimist"
	// RoleSkeptic 守旧派：从记忆信息出发，关注风险和历史教训
	RoleSkeptic DelegateRole = "skeptic"
	// RoleCritic 反对者：同时挑两派逻辑漏洞、证据链断裂、未考虑的问题
	RoleCritic DelegateRole = "critic"
	// RoleSynthesizer 整合者：主持辩论、判断收敛、综合报告
	RoleSynthesizer DelegateRole = "synthesizer"
)

// DebateRound 一轮辩论记录
type DebateRound struct {
	Round       int
	Optimist    string // 乐观派发言
	Skeptic     string // 审慎派发言
	Critic      string // 反对者挑刺
	Synthesizer string // 整合者判断
	Converged   bool   // 本轮是否收敛
}

// AssemblyState 大会状态
type AssemblyState struct {
	OriginalQuery            string
	Rounds                   []DebateRound
	CurrentRound             int
	Converged                bool
	ResearchData             *ResearchData // 深度搜索采集的结构化研究数据
	CriticEmptyRounds        int           // 反对者连续无新问题的轮数
	SupplementarySearchCount int           // 已执行的补充搜索次数
}

// Assembly 大会辩论系统
type Assembly struct {
	depth           *DepthSearcher
	webpageSearcher *WebpageSearcher // 补充搜索用普通搜索级别
	llmProvider     Provider
	memProvider     MemoryProvider
	cfg             DepthConfig
	debugLog        func(format string, args ...interface{}) // 诊断日志回调
	logFile         io.Writer                                // 辩论日志文件
	logMu           sync.Mutex                               // 日志写入锁
}
