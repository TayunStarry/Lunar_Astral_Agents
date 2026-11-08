package creation

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
	Agent string `json:"agent"` // writer / engineer / researcher / runner / painter
	Task  string `json:"task"`  // 自然语言执行目标与操作要求
	// Paint 图片绘制交接信息；agent=painter 时必填，避免把绘图退化成自然语言描述
	Paint *paintBrief `json:"paint,omitempty"`
	// Expect 规划预期：该步骤完成后应满足的客观事实，用于自动审查
	Expect []fileflow.Expect `json:"expect,omitempty"`
}

// paintBrief 交给图片绘制智能体的结构化绘图需求。
type paintBrief struct {
	// Prompt 画面描述（主体、构图、光线、细节）
	Prompt string `json:"prompt"`
	// Ratio 尺寸 / 画幅（如 1:1、16:9、3:4）
	Ratio string `json:"ratio,omitempty"`
	// Style 风格约束
	Style string `json:"style,omitempty"`
	// References 参考资源（图片句柄 / 文档锚点 / 本机图片路径）；第一张作为图生图基座
	References []string `json:"references,omitempty"`
	// Change 基于参考图要改什么（如换背景、换服饰、改画风）；有参考图时必填
	Change string `json:"change,omitempty"`
	// Output 输出去向说明（资源目录或期望句柄），可空
	Output string `json:"output,omitempty"`
}

// PaintRequest 创作链交给图片绘制智能体的完整需求。
type PaintRequest struct {
	// Prompt 画面描述
	Prompt string
	// Ratio 尺寸 / 画幅
	Ratio string
	// Style 风格约束
	Style string
	// References 参考资源（图片句柄 / 文档锚点 / 本机图片路径）
	References []string
	// Change 基于参考图要改什么
	Change string
	// Output 输出去向说明（资源目录或期望句柄），可空
	Output string
	// Task 调度者给出的原始任务描述，供图片绘制智能体理解上下文
	Task string
}

// PaintResult 图片绘制的产出。
type PaintResult struct {
	// Summary 绘制过程或结果摘要
	Summary string
	// Handles 产出的图片资源句柄
	Handles []string
}

// SubmitRequest 资源库提交的创作任务。
type SubmitRequest struct {
	// Mode assigned（有明确描述）或 random（随机目标池自主选题）
	Mode string
	// Description 自然语言描述；random 模式可空
	Description string
	// Handles 目标资源句柄
	Handles []string
}

// TaskView 创作任务对外可见的状态与结果。
type TaskView struct {
	TaskID    string   `json:"task_id"`
	Mode      string   `json:"mode"`
	Status    string   `json:"status"`
	Stage     string   `json:"stage,omitempty"`
	Progress  int      `json:"progress"`
	Summary   string   `json:"summary,omitempty"`
	Handles   []string `json:"handles,omitempty"`
	Error     string   `json:"error,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// taskState 内存中的任务记录。
type taskState struct {
	view TaskView
}

// ProgressEvent 创作流程进度事件（供任务管理与前端查询）。
type ProgressEvent struct {
	// Status queued / planning / executing / completed / failed
	Status string
	// Stage 当前阶段的可读说明
	Stage string
	// Progress 0-100
	Progress int
	// Summary 阶段结果摘要
	Summary string
	// Handles 当前已产出的资源句柄
	Handles []string
}

// pathSnapshotNode 路径树快照中的一个条目（相对 LocalDir）。
type pathSnapshotNode struct {
	Rel string
	Dir bool
}

// pathRootCount 单个根目录下的文件与目录数量。
type pathRootCount struct {
	Files int
	Dirs  int
}

// pathSnapshotCache 路径树快照缓存：Key 为 LocalDir，BuiltAt 留零值即视为失效。
type pathSnapshotCache struct {
	Key     string
	Text    string
	BuiltAt time.Time
}

// planResult 调度者输出的本轮计划。
type planResult struct {
	Steps []planStep `json:"steps"`
	Memos []string   `json:"memos,omitempty"`
}

// journal 创作事件的规划日志视图：由统一库 creation.db 的上一轮调用记录构建。
// Directives 保存本轮创作参考（来自 life_planning 的字符串变量），一次性消费，不落库。
// Changes 为上一轮的文件内容级变更（来自 change 表），完整内容可按路径查库。
type journal struct {
	Version     int                     `json:"version"`
	Mode        string                  `json:"mode,omitempty"`
	Directives  []string                `json:"directives,omitempty"`
	LastPlans   []planStep              `json:"last_plans"`
	Memos       []string                `json:"memos"`
	References  []string                `json:"references"`
	Errors      []string                `json:"errors"`
	LastSummary string                  `json:"last_summary"`
	Changes     []fileflow.ChangeRecord `json:"changes,omitempty"`
	UpdatedAt   int64                   `json:"updated_at"`
}

// stepResult 单个调度项的执行结果。
type stepResult struct {
	Agent   string `json:"agent"`
	Task    string `json:"task"`
	Result  string `json:"result,omitempty"`
	Skipped bool   `json:"skipped"`
	Error   string `json:"error,omitempty"`
	// Handles 本步骤产出的资源句柄（图片绘制步骤会回填）
	Handles []string `json:"handles,omitempty"`
	// ReviewFailures 本步骤内审查失败并回滚的上下文
	ReviewFailures []string `json:"review_failures,omitempty"`
}

// changeGroup 同一操作（同一 flow + seq）在 change 表中的一组差异块。
type changeGroup struct {
	seq     int
	kind    string
	path    string
	records []fileflow.ChangeRecord
}

// session 一次创作运行的临时状态：收集参考资料与错误，供轮次归档。
type session struct {
	callID    string
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
	// paintHandles 图片绘制智能体本轮产出的资源句柄
	paintHandles []string
	// observe 进度观察器；为空表示不对外推送进度
	observe func(ProgressEvent)
}

// report 向进度观察器推送一次事件（观察器为空时忽略）。
func (s *session) report(event ProgressEvent) {
	if s.observe == nil {
		return
	}
	if len(event.Handles) == 0 {
		event.Handles = append([]string(nil), s.paintHandles...)
	}
	s.observe(event)
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
