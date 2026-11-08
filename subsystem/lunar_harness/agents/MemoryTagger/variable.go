package MemoryTagger

// variable.go 记忆标签智能体的常量定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属 ID
	AgentID = "memory_tagger"
	// modelRole 推理实况角色标签
	modelRole = agentinfo.NameMemoryTagger
)

const (
	// maxRetries 标签生成最大重试次数
	maxRetries = 3
	// retryWait 两次重试之间的等待
	retryWait = time.Second
	// maxTokens 标签生成输出上限
	maxTokens = 512
	// temperature 标签生成采样温度
	temperature = 0.3
	// callTimeout 单次模型调用超时
	callTimeout = 120 * time.Second
)

// 图片识别取向（与记忆库对外契约保持一致）
const (
	recognitionAuto       = "auto"
	recognitionEmotion    = "emotion"
	recognitionText       = "text"
	recognitionColor      = "color"
	recognitionAppearance = "appearance"
	recognitionSpecies    = "species"
	recognitionPosture    = "posture"
	recognitionCustom     = "custom"
)

const (
	// textSystemPrompt 文本标签生成的系统提示词
	textSystemPrompt = "你是一个内容标签生成助手。请为以下文本生成标签，描述其核心主题和关键信息。用中文输出，严格以JSON数组格式返回，不要包含任何其他内容。\n\n" +
		"示例输出：[\"人工智能\",\"机器学习\",\"深度学习\",\"神经网络\"]"
	// mediaFrameNote 动态图帧序列的补充说明
	mediaFrameNote = "\n\n本次输入不是单张静态图片，而是同一段动画（GIF/动图）按时间顺序抽取的采样帧序列。" +
		"请综合各帧的画面内容与帧间的动作/变化过程生成标签，标签需覆盖动作与过程维度。"
)
