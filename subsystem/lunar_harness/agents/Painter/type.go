package Painter

// type.go 绘制者智能体的类型定义。
//
// 绘制者只持有自身的提示词、工具声明与编排状态；渲染、推送、视觉副本与
// 图片字节解析等宿主能力经 Env 注入，避免把月华的传输管线细节带进智能体内部。

import (
	"context"

	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// Env 绘制者依赖的宿主能力。
type Env interface {
	// PushImages 推送图片并返回规范化后的可查看地址
	PushImages(images []string) []string
	// VisionCopy 生成视觉副本并返回像素宽高
	VisionCopy(relPath, tag string) (width, height int, err error)
	// VisionReference 生成思维链消费的视觉读取引用
	VisionReference(handle, blobPath string) string
	// ReadImageBytes 把来源地址解析为图片字节
	ReadImageBytes(source string) ([]byte, error)
}

// Result 一次绘制任务的结果。
type Result struct {
	// Summary 回传给对话者的作品说明
	Summary string
	// Handles 本次任务产出的图片句柄（无产出时为空）
	Handles []string
}

// Reference 交给绘制者的参考图。
type Reference struct {
	Handle string
	Source string
	Desc   string
	// Extras 附加参考图：仅用于视觉对照，图生图基座始终是第一张
	Extras []Reference
	// Explicit 显式请求：true 且没有可用参考图时禁止沿用运行时画作
	Explicit bool
}

// ReferenceState 任务级参考图状态。
type ReferenceState struct {
	Image  string
	Handle string
	Vision string
	Width  int
	Height int
}

// DimensionPlan 输出尺寸计划。
type DimensionPlan struct {
	Width  int
	Height int
	Origin string
}

// GenerateImageParams 图像生成参数。
type GenerateImageParams struct {
	Prompt          string
	NegativePrompt  string
	BatchSize       int
	Width           int
	Height          int
	Steps           int
	Strength        float64
	CfgScale        float64
	Seed            int64
	InitImg         string
	SuperResolution bool
}

// PaintingDetail 绘画作品详情。
type PaintingDetail struct {
	ToolName      string
	PromptSummary string
	Character     string
	Chibi         bool
	Expression    string
	Posture       string
	Environment   string
	Roles         []string
	Planning      string
}

// CustomCharacterProfile 自定义角色档案。
type CustomCharacterProfile struct {
	Name        string `json:"name"`
	Gender      string `json:"gender"`
	Appearance  string `json:"appearance"`
	OutfitFull  string `json:"outfitFull,omitempty"`
	OutfitChibi string `json:"outfitChibi,omitempty"`
}

// CharacterProfile 角色档案（内置 + 自定义统一视图）。
type CharacterProfile struct {
	CustomCharacterProfile
	Builtin bool
}

// AppearanceParts 外观模板占位符填充参数。
type AppearanceParts struct {
	Expression  string
	Posture     string
	Outfit      string
	Environment string
}

// PortraitPreset 立绘预设。
type PortraitPreset struct {
	Kind         string
	Chibi        bool
	Label        string
	Template     string
	Base         string
	Outfit       string
	Gender       string
	DefaultRatio string
}

// PromptDocument 按标题路径读取的提示词文档。
type PromptDocument struct {
	Sections map[string]string
	File     string
}

// PromptLibrary 绘制者提示词库。
type PromptLibrary struct {
	Common     *PromptDocument
	Characters *PromptDocument
	Templates  map[string]string
	Builtin    map[string]CharacterProfile
}

// chatMessage 绘制者请求中的消息项（统一复用基座类型）。
type chatMessage = agentbase.Message

// chatCall 一次工具调用的函数名与原始参数。
type chatCall struct {
	Name      string
	Arguments any
}

// session 单次绘制任务的编排状态。
type session struct {
	ctx          context.Context
	env          Env
	prompts      *PromptLibrary
	custom       map[string]CustomCharacterProfile
	reference    ReferenceState
	extras       []ReferenceState
	textOnly     bool
	forceImg2Img bool
	fromRuntime  bool
	resultPath   string
	runtime      []chatMessage
	artwork      ReferenceState
	task         any
}

// runtimeState 跨任务保留的运行时画作与最近作品路径。
type runtimeState struct {
	artwork ReferenceState
	path    string
	history []chatMessage
}
