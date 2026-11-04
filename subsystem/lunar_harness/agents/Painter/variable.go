package Painter

// variable.go 绘制者的常量与全局状态。

import (
	"regexp"
	"sync"
)

// ==== 身份与提示词 ====

const (
	// module 日志模块名
	module = "Painter"
	// agentID 上报、数据导出与智能修复共用的智能体标识
	agentID = "painter"
	// roleLabel 实况与请求头中的角色标签
	roleLabel = "绘制者"
	// rolePromptPath 绘制者角色提示词
	rolePromptPath = "prompts/painterRole.md"
	// commonPromptPath 通用绘画提示词（负面词、合照模板、随机池）
	commonPromptPath = "prompts/painterCommon.md"
	// charactersPromptPath 角色外观提示词库
	charactersPromptPath = "prompts/painterCharacters.md"
	// charactersTable 自定义角色库所在逻辑表
	charactersTable = "painter_characters"
)

// ==== 编排参数 ====

const (
	// maxIterations 单次任务最多推理轮数
	maxIterations = 3
	// contextLimit 跨任务保留的上下文条数上限
	contextLimit = 40
	// retryAttempts 模型输出陷入重复时的总尝试次数
	retryAttempts = 2
	// fileReadPath 前端可查看原图的读取端点
	fileReadPath = "/file/read/"
	// imageKind 图片资源类型码
	imageKind = "I"
	// listLimit 列出图片句柄的数量上限
	listLimit = 100
)

// ==== 画面尺寸 ====

const (
	sideBase      = 1024
	sideMax       = 2816
	sideAlign     = 32
	refSideMin    = 512
	defaultBudget = 1.6
	budgetSlack   = 1.15
	defaultRatio  = "1:1"
)

var ratioBase = map[string]struct {
	ratio      float64
	horizontal bool
}{
	"1:1":  {1, true},
	"4:3":  {4.0 / 3.0, true},
	"3:4":  {4.0 / 3.0, false},
	"16:9": {16.0 / 9.0, true},
	"9:16": {16.0 / 9.0, false},
	"3:2":  {3.0 / 2.0, true},
	"2:3":  {3.0 / 2.0, false},
}

// ==== 提示词清洗 ====

var (
	regexpCommaComma   = regexp.MustCompile(`[，,]\s*[，,]+`)
	regexpCommaPeriod  = regexp.MustCompile(`[，,]\s*。`)
	regexpPeriodPeriod = regexp.MustCompile(`。\s*。`)
	regexpSpaceRun     = regexp.MustCompile(`\s{2,}`)
)

// ==== 运行时状态 ====

// stateMu 串行化绘制任务并保护跨任务状态。
var stateMu sync.Mutex

// state 跨任务保留的运行时画作、最近作品路径与上下文。
var state runtimeState
