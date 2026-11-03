package Creator

// type.go 创作体系的类型定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/fileflow"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// ==== 模型调用类型 ====

// toolDefinition 发给模型的函数工具定义（统一复用基座类型）。
type toolDefinition = agentbase.ToolDefinition

// toolFunction 函数描述与参数 Schema（统一复用基座类型）。
type toolFunction = agentbase.ToolFunctionDef

// ==== 工具执行类型 ====

// toolOutcome 工具执行结果：JSON 回填 + 给最终总结用的短句 + 可归档的参考资料。
type toolOutcome struct {
	JSON      string
	Summary   string
	Reference string
}

// toolHandlerFunc 工具分发；done 为 true 时结束当前推理循环。
type toolHandlerFunc func(name string, args map[string]any) (outcome toolOutcome, done bool, err error)

// ==== 规划与日志类型 ====

// planStep 一个按严格顺序执行的调度项。
type planStep struct {
	Agent string `json:"agent"` // writer / engineer / researcher / runner
	Task  string `json:"task"`  // 自然语言执行目标与操作要求
	// Expect 规划预期：该步骤完成后应满足的客观事实，用于自动审查
	Expect []fileflow.Expect `json:"expect,omitempty"`
}

// planResult 调度者输出的本轮计划。
type planResult struct {
	Steps []planStep `json:"steps"`
	Memos []string   `json:"memos,omitempty"`
}

// journal 创作事件的规划日志：记录上一轮的全部规划、备忘、参考资料与错误。
// Directives 保存本轮创作参考（来自 life_planning 的字符串变量），一次性消费。
// 操作记录不再写入日志：改由 engine/fileflow 的增量库按需查询内容级差异。
type journal struct {
	Version     int        `json:"version"`
	Mode        string     `json:"mode,omitempty"`
	Directives  []string   `json:"directives,omitempty"`
	LastPlans   []planStep `json:"last_plans"`
	Memos       []string   `json:"memos"`
	References  []string   `json:"references"`
	Errors      []string   `json:"errors"`
	LastSummary string     `json:"last_summary"`
	UpdatedAt   int64      `json:"updated_at"`
}

// stepResult 单个调度项的执行结果。
type stepResult struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Result  string `json:"result,omitempty"`
	Skipped bool   `json:"skipped"`
	Error   string `json:"error,omitempty"`
	// ReviewFailures 本步骤内审查失败并回滚的上下文
	ReviewFailures []string `json:"review_failures,omitempty"`
}

// session 一次创作运行的临时状态：收集参考资料与错误，供日志归档。
type session struct {
	refs      []string
	errs      []string
	memos     []string
	plan      planResult
	planReady bool
	mode      fileflow.Mode
	flowID    string
	flow      *fileflow.Context
	current   *planStep
	stepIndex int
}

// ==== 路由与文件操作流程类型 ====

// routeKind 承接操作的智能体。
type routeKind string

// routeDecision 一次路由决策。
type routeDecision struct {
	// Kind 承接智能体
	Kind routeKind
	// Abs 目标绝对路径（去掉压缩包条目后缀）
	Abs string
	// Rel 相对本地目录的路径
	Rel string
	// IsDir 目标是否为目录
	IsDir bool
	// Large 目录 / 文件是否达到大型阈值
	Large bool
	// Entry 压缩包内条目（目标写成 pkg.zip/entry 时提取）
	Entry string
	// Reason 路由依据（供流程记录与模型理解）
	Reason string
}

// mutationResult 统一封装各能力智能体的写操作结果。
type mutationResult struct {
	// OK 操作是否成功
	OK bool
	// Path 操作后的目标路径
	Path string
	// Summary 操作简述
	Summary string
	// Handles 本次涉及的资源句柄
	Handles []string
	// Detail 操作细节
	Detail map[string]any
}

// ==== 文件与代码索引类型 ====

// codeSymbol 代码索引条目。
type codeSymbol struct {
	Name      string `json:"name"`
	Signature string `json:"signature,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// codeIndex 一个代码文件的三类结构化索引。
type codeIndex struct {
	Variables []codeSymbol `json:"variables"`
	Functions []codeSymbol `json:"functions"`
	Classes   []codeSymbol `json:"classes"`
}

// fileEntry 目录列表条目。
type fileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// ==== 脚本运行类型 ====

// scriptRunResult 一次 LTP9 脚本运行的结果。
type scriptRunResult struct {
	Package    string   `json:"package"`
	Loaded     bool     `json:"loaded"`
	Running    bool     `json:"running"`
	Authorized []string `json:"authorized"`
	Exports    []string `json:"exports"`
	Result     any      `json:"result,omitempty"`
	Error      string   `json:"error,omitempty"`
}

// docInfo DTS 开发文档信息。
type docInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// nowUnix 当前时间。
func nowUnix() int64 { return time.Now().Unix() }
