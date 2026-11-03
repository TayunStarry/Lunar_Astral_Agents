package fileflow

// type.go 文件操作流程的类型定义：操作种类、规划预期、快照、审查结果与流程上下文。

import "fmt"

// ==== 生成模式 ====

// Mode 流程运行模式（决定日志留存策略）。
type Mode string

const (
	// ModeCreator 自由创作模式：完整流程记录全程保存。
	ModeCreator Mode = "creator"
	// ModeAssigned 对话者指派模式：只保留历史操作记录供下一次任务使用。
	ModeAssigned Mode = "assigned"
)

// ==== 操作种类 ====

const (
	// OpRead 读取文件 / 目录
	OpRead = "read"
	// OpList 列出目录
	OpList = "list"
	// OpWrite 整篇写入
	OpWrite = "write"
	// OpCreate 新建文件
	OpCreate = "create"
	// OpReplace 按行置换
	OpReplace = "replace"
	// OpDelete 删除文件
	OpDelete = "delete"
	// OpMove 移动 / 重命名
	OpMove = "move"
	// OpCopy 复制
	OpCopy = "copy"
	// OpArchiveRead 压缩包 / GGUF 读取
	OpArchiveRead = "archive_read"
	// OpArchiveWrite 压缩包写入（创建 / 删除 / 增删条目）
	OpArchiveWrite = "archive_write"
)

// Mutating 判断操作是否会改变文件内容。
func Mutating(op string) bool {
	switch op {
	case OpWrite, OpCreate, OpReplace, OpDelete, OpMove, OpCopy, OpArchiveWrite:
		return true
	}
	return false
}

// ==== 规划预期 ====

// Expect 一条可确定性比对的规划预期。
type Expect struct {
	// Path 目标路径（相对 /document、相对本地目录或绝对路径）
	Path string `json:"path"`
	// Exists 期望是否存在（nil 表示不检查存在性）
	Exists *bool `json:"exists,omitempty"`
	// Contains 期望包含的文本片段
	Contains []string `json:"contains,omitempty"`
	// NotContains 期望不包含的文本片段
	NotContains []string `json:"not_contains,omitempty"`
	// MinBytes 期望的最小字节数
	MinBytes *int64 `json:"min_bytes,omitempty"`
	// MaxBytes 期望的最大字节数
	MaxBytes *int64 `json:"max_bytes,omitempty"`
}

// ==== 快照 ====

// TreeFile 目录快照中的一个文件。
type TreeFile struct {
	// Rel 相对目录根的路径
	Rel string `json:"rel"`
	// Size 字节数
	Size int64 `json:"size,omitempty"`
	// SHA256 内容哈希
	SHA256 string `json:"sha256,omitempty"`
	// Blob 快照副本路径（相对本地目录）
	Blob string `json:"blob,omitempty"`
}

// Entry 快照中的单个路径条目。
type Entry struct {
	// Abs 绝对路径
	Abs string `json:"abs"`
	// Rel 相对本地目录的路径
	Rel string `json:"rel"`
	// Existed 操作前是否存在
	Existed bool `json:"existed"`
	// IsDir 操作前是否为目录
	IsDir bool `json:"is_dir,omitempty"`
	// Size 操作前字节数
	Size int64 `json:"size,omitempty"`
	// SHA256 操作前内容哈希
	SHA256 string `json:"sha256,omitempty"`
	// Blob 快照副本路径（相对本地目录）
	Blob string `json:"blob,omitempty"`
	// Dirs 目录快照内的子目录（相对目录根）
	Dirs []string `json:"dirs,omitempty"`
	// Files 目录快照内的文件
	Files []TreeFile `json:"files,omitempty"`
}

// Snapshot 一次操作前的版本快照。
type Snapshot struct {
	// FlowID 流程 ID
	FlowID string `json:"flow_id"`
	// Seq 流程内操作序号（1 基）
	Seq int `json:"seq"`
	// Op 操作种类
	Op string `json:"op"`
	// Target 调用方给出的目标路径
	Target string `json:"target"`
	// Dir 快照目录（相对本地目录）
	Dir string `json:"dir"`
	// Entries 路径条目
	Entries []Entry `json:"entries"`
	// CreatedAt 创建时间（Unix 秒）
	CreatedAt int64 `json:"created_at"`
}

// rollbackLog 回滚结果记录（随快照落盘，供追溯）。
type rollbackLog struct {
	// OK 回滚是否全部成功
	OK bool `json:"ok"`
	// Errors 失败条目
	Errors []string `json:"errors,omitempty"`
	// At 回滚时间（Unix 秒）
	At int64 `json:"at"`
}

// ==== 审查 ====

// Failure 一条审查失败原因。
type Failure struct {
	// Path 目标路径
	Path string `json:"path"`
	// Reason 失败原因
	Reason string `json:"reason"`
}

// Review 一次操作后的审查结果。
type Review struct {
	// OK 是否与规划预期一致
	OK bool `json:"ok"`
	// Checked 实际比对的预期条数
	Checked int `json:"checked"`
	// Failures 失败原因
	Failures []Failure `json:"failures,omitempty"`
	// RolledBack 是否已回滚
	RolledBack bool `json:"rolled_back"`
	// RollbackError 回滚失败原因（非空表示回滚未成功）
	RollbackError string `json:"rollback_error,omitempty"`
	// Snapshot 快照目录（相对本地目录）
	Snapshot string `json:"snapshot,omitempty"`
	// Context 交给下一轮流程节点的自然语言上下文
	Context string `json:"context,omitempty"`
}

// Failed 审查是否失败且尚未回滚（回滚情况由 RolledBack / RollbackError 表示）。
func (r Review) Failed() bool { return !r.OK }

// Summary 返回一行审查结论。
func (r Review) Summary() string {
	if r.OK {
		return fmt.Sprintf("审查通过（%d 条预期）", r.Checked)
	}
	if r.RollbackError != "" {
		return fmt.Sprintf("审查未通过：%s；回滚失败：%s", r.ReasonText(), r.RollbackError)
	}
	if r.RolledBack {
		return fmt.Sprintf("审查未通过：%s；已回滚到操作前版本", r.ReasonText())
	}
	return "审查未通过：" + r.ReasonText()
}

// ReasonText 把失败原因拼成一段文本。
func (r Review) ReasonText() string {
	text := ""
	for index, failure := range r.Failures {
		if index > 0 {
			text += "；"
		}
		text += failure.Reason
	}
	if text == "" {
		text = "未命中的预期"
	}
	return text
}

// ==== 流程上下文 ====

// Record 一次文件操作的记录（统一汇入流程上下文）。
type Record struct {
	// Seq 流程内序号
	Seq int `json:"seq"`
	// Step 规划步骤序号（0 表示规划阶段外）
	Step int `json:"step,omitempty"`
	// Op 操作种类
	Op string `json:"op"`
	// Target 目标路径
	Target string `json:"target"`
	// Route 实际承接该操作的智能体
	Route string `json:"route"`
	// Summary 操作简述
	Summary string `json:"summary,omitempty"`
	// Handles 本次涉及的资源句柄
	Handles []string `json:"handles,omitempty"`
	// Review 审查结果（读操作无审查）
	Review *Review `json:"review,omitempty"`
	// Error 执行失败原因
	Error string `json:"error,omitempty"`
	// At 操作时间（Unix 秒）
	At int64 `json:"at"`
}

// ==== 内容级增量记录 ====

// 增量种类：直接对应可回退的变更形态。
const (
	// DeltaCreate 新增文件 / 目录
	DeltaCreate = "create"
	// DeltaModify 修改已有文件（含新增、删除、替换的文本块）
	DeltaModify = "modify"
	// DeltaDelete 删除文件 / 目录
	DeltaDelete = "delete"
	// DeltaRename 移动 / 重命名
	DeltaRename = "rename"
	// DeltaDirBatch 目录级批量写
	DeltaDirBatch = "dir_batch"
	// DeltaArchive 压缩包写
	DeltaArchive = "archive_write"
)

// 审查状态。
const (
	// ReviewPassed 审查通过
	ReviewPassed = "passed"
	// ReviewRolledBack 审查未通过但已回滚
	ReviewRolledBack = "rolled_back"
	// ReviewFailed 审查未通过且未能回滚
	ReviewFailed = "failed"
)

// 操作记录状态。
const (
	// OpStatusApplied 操作已生效
	OpStatusApplied = "applied"
	// OpStatusRolledBack 已按记录 / 快照回退
	OpStatusRolledBack = "rolled_back"
	// OpStatusRollbackFailed 回退失败
	OpStatusRollbackFailed = "rollback_failed"
)

// Delta 一条内容级增量（一次操作内可有多条）。
type Delta struct {
	// Kind 增量种类（create / modify / delete / rename / dir_batch / archive_write）
	Kind string `json:"kind"`
	// PathRel 目标相对路径
	PathRel string `json:"path_rel"`
	// PathAbs 目标绝对路径
	PathAbs string `json:"path_abs"`
	// SourceRel 重命名 / 移动的来源相对路径
	SourceRel string `json:"source_rel,omitempty"`
	// Locator 可读定位（如「第 12-14 行」「字节 0-4096」）
	Locator string `json:"locator,omitempty"`
	// LineFrom 变更在操作后文件中的起始行（1 基，插入点取插入位置）
	LineFrom int `json:"line_from,omitempty"`
	// LineTo 变更在操作后文件中的结束行（含；插入时等于 LineFrom-1）
	LineTo int `json:"line_to,omitempty"`
	// OldText 替换前片段（按上限截断）
	OldText string `json:"old_text,omitempty"`
	// NewText 替换后片段（按上限截断）
	NewText string `json:"new_text,omitempty"`
	// OldHash 变更前内容哈希
	OldHash string `json:"old_hash,omitempty"`
	// NewHash 变更后内容哈希
	NewHash string `json:"new_hash,omitempty"`
	// OldSize 变更前字节数
	OldSize int64 `json:"old_size,omitempty"`
	// NewSize 变更后字节数
	NewSize int64 `json:"new_size,omitempty"`
	// Binary 是否为二进制变更（不存文本片段）
	Binary bool `json:"binary,omitempty"`
	// BinaryNote 二进制可读差异结论（追加 / 截断 / 整文件替换等）
	BinaryNote string `json:"binary_note,omitempty"`
	// Truncated 片段是否因超出上限被截断（截断的条目回退时改用快照兜底）
	Truncated bool `json:"truncated,omitempty"`
}

// OpInput 一次操作记录的输入：快照 + 执行结果 + 审查结论。
type OpInput struct {
	// FlowID 流程 ID
	FlowID string
	// Seq 流程内序号
	Seq int
	// Step 规划步骤序号（0 表示规划阶段外）
	Step int
	// Mode 运行模式
	Mode Mode
	// Op 操作种类
	Op string
	// Target 调用方给出的目标路径
	Target string
	// PathRel 目标相对路径
	PathRel string
	// PathAbs 目标绝对路径
	PathAbs string
	// Agent 执行智能体（索引元数据）
	Agent string
	// Tool 工具名（索引元数据）
	Tool string
	// Detail 各能力智能体的执行细节（压缩包条目等）
	Detail map[string]any
	// Snapshot 操作前快照
	Snapshot Snapshot
	// Review 审查结果
	Review Review
}

// BuiltOp 已生成但尚未落库的操作增量（审查前采集，审查后提交，避免回滚后采集到空差异）。
type BuiltOp struct {
	// Input 操作输入
	Input OpInput
	// Deltas 内容增量
	Deltas []Delta
	// BeforeHash 操作前哈希
	BeforeHash string
	// AfterHash 操作后哈希
	AfterHash string
	// BeforeSize 操作前字节数
	BeforeSize int64
	// AfterSize 操作后字节数
	AfterSize int64
}

// OpRecord 从数据库读出的完整操作记录（含内容增量）。
type OpRecord struct {
	// ID 数据库主键
	ID int64 `json:"id"`
	// FlowID 流程 ID
	FlowID string `json:"flow_id"`
	// Seq 流程内序号
	Seq int `json:"seq"`
	// Step 规划步骤序号
	Step int `json:"step,omitempty"`
	// Mode 运行模式
	Mode Mode `json:"mode,omitempty"`
	// Op 操作种类
	Op string `json:"op"`
	// DeltaKind 内容增量的语义类型（创建 / 修改 / 删除 / 重命名 / 目录批量 / 压缩包）
	DeltaKind string `json:"delta_kind,omitempty"`
	// Target 调用方给出的目标路径
	Target string `json:"target,omitempty"`
	// PathRel 目标相对路径
	PathRel string `json:"path_rel,omitempty"`
	// PathAbs 目标绝对路径
	PathAbs string `json:"path_abs,omitempty"`
	// Agent 执行智能体
	Agent string `json:"agent,omitempty"`
	// Tool 工具名
	Tool string `json:"tool,omitempty"`
	// Snapshot 操作前快照目录（相对本地目录）
	Snapshot string `json:"snapshot,omitempty"`
	// BeforeSHA256 操作前哈希
	BeforeSHA256 string `json:"before_sha256,omitempty"`
	// AfterSHA256 操作后哈希
	AfterSHA256 string `json:"after_sha256,omitempty"`
	// BeforeSize 操作前字节数
	BeforeSize int64 `json:"before_size"`
	// AfterSize 操作后字节数
	AfterSize int64 `json:"after_size"`
	// ReviewStatus 审查状态（passed / rolled_back / failed）
	ReviewStatus string `json:"review_status,omitempty"`
	// ReviewDetail 审查结论
	ReviewDetail string `json:"review_detail,omitempty"`
	// RollbackOK 是否已成功回退
	RollbackOK bool `json:"rollback_ok"`
	// RollbackError 回退失败原因
	RollbackError string `json:"rollback_error,omitempty"`
	// Status 操作状态（applied / rolled_back / rollback_failed）
	Status string `json:"status,omitempty"`
	// Deltas 内容增量
	Deltas []Delta `json:"deltas,omitempty"`
	// At 记录时间（Unix 秒）
	At int64 `json:"at"`
}

// fileState 路径在某一时刻的状态；仅对不超过读取上限的文件保留原始内容用于差异计算。
type fileState struct {
	// Exists 是否存在
	Exists bool
	// IsDir 是否为目录
	IsDir bool
	// Size 字节数（目录为文件总字节）
	Size int64
	// Hash 内容哈希（目录为空）
	Hash string
	// Data 文件原始内容（目录或超限时为空）
	Data []byte
	// IsText 是否为文本文件
	IsText bool
	// Tree 目录内文件（相对路径 → 文件信息）
	Tree map[string]TreeFile
}

// RollbackResult 按流程回退到指定步骤的结果。
type RollbackResult struct {
	// OK 是否全部回退成功
	OK bool `json:"ok"`
	// FlowID 流程 ID
	FlowID string `json:"flow_id"`
	// TargetSeq 回退目标步骤（seq > TargetSeq 全部回退）
	TargetSeq int `json:"target_seq"`
	// Reverted 已回退的操作条数
	Reverted int `json:"reverted"`
	// Errors 失败条目
	Errors []string `json:"errors,omitempty"`
}

// Context 一次流程的上下文：操作记录与错误，供下一节点与日志使用。
type Context struct {
	// Mode 运行模式
	Mode Mode `json:"mode"`
	// FlowID 流程 ID
	FlowID string `json:"flow_id"`
	// Records 操作记录
	Records []Record `json:"records"`
	// Errors 流程错误
	Errors []string `json:"errors,omitempty"`
}

// NewContext 创建流程上下文。
func NewContext(mode Mode, flowID string) *Context {
	return &Context{Mode: mode, FlowID: flowID, Records: []Record{}}
}

// Add 追加一条操作记录。
func (c *Context) Add(record Record) {
	if c == nil {
		return
	}
	c.Records = append(c.Records, record)
}

// AddError 追加一条流程错误。
func (c *Context) AddError(text string) {
	if c == nil || text == "" {
		return
	}
	c.Errors = append(c.Errors, text)
}

// LatestFailure 返回最近一条审查失败 / 执行失败的上下文文本。
func (c *Context) LatestFailure() string {
	if c == nil {
		return ""
	}
	for index := len(c.Records) - 1; index >= 0; index-- {
		record := c.Records[index]
		if record.Review != nil && !record.Review.OK {
			return record.Review.Context
		}
		if record.Error != "" {
			return record.Error
		}
	}
	return ""
}
