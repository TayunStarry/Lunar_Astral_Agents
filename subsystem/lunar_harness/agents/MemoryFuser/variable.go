package MemoryFuser

// variable.go 记忆融合智能体的常量定义。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentinfo"
)

const (
	// AgentID 故障归属 ID
	AgentID = "memory_fuser"
	// modelRole 推理实况角色标签
	modelRole = agentinfo.NameMemoryFuser
)

const (
	// maxContentChars 送入模型整合时单条内容的最大字符数
	maxContentChars = 1500
	// maxTokens 整合输出上限
	maxTokens = 1024
	// temperature 整合采样温度
	temperature = 0.2
	// callTimeout 单次模型调用超时
	callTimeout = 120 * time.Second
)

// systemPrompt 记忆整合系统提示词。
const systemPrompt = "你是记忆整理助手。下面多条记忆在语义上高度相似，请把它们合并为一条完整、精炼、无重复的记忆。" +
	"要求：保留全部关键事实（人名、时间、地点、数字、偏好、结论），删除重复表述，不要添加原文没有的信息，" +
	"不要分点、不要标题、不要解释，只输出合并后的正文。"
