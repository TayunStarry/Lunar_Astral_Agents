package memory

import "errors"

// MediaSubdir 记忆库图片在 multimedia 下的专属子目录：
// 图片实体统一落在 multimedia/memory/<集合名>/，与生成图、背景图、截图等隔离，便于清理。
const MediaSubdir = "memory"

// v5 记忆库配置常量 — 文本/图片/音频混合存储，仅内容嵌入向量
const (
	CurrentVersion     = 5    // 当前数据格式版本号（v5: 混合存储 + 仅内容嵌入）
	DocumentsChunkSize = 100  // 文档分块大小（条/块，各类型统一；100 条一块）
	DedupThreshold     = 0.98 // 内容嵌入向量去重阈值（余弦相似度，判定重复记忆）
	DocRankThreshold   = 0.30 // 文档内容余弦得分门槛（低于此值不返回）
	MaxTagRetries      = 3    // 图片多模态标签生成最大重试次数

	// FastSearchCollectionLimit 快速检索的集合范围上限（含最相近的那个）
	// 首集合命中后只补查第 2~5 近的集合；首集合未命中则回退扫描其余全部集合。
	FastSearchCollectionLimit = 5
)

// 记忆类型 — 查询与列举时按类型过滤，不再按集合名过滤
const (
	TypeText  = "text"  // 文本记忆（对话原文、知识片段）
	TypeImage = "image" // 图像记忆（表情包、截图、图片素材）
	TypeAudio = "audio" // 音频记忆（语音、音乐、环境声）
)

// 记忆权重融合（把超高相似度的文本条目合并为一条整合记忆）参数
const (
	// FuseDefaultThreshold 默认相似度阈值（余弦）；同簇条目与该簇代表条目需达到此值
	FuseDefaultThreshold = 0.90
	// FuseMinThreshold 允许的最小阈值，低于此值会被夹回默认值（避免把无关记忆混在一起）
	FuseMinThreshold = 0.50
	// FuseAutoModelChars auto 策略下改用模型整合的正文字符数门槛
	FuseAutoModelChars = 600
	// FuseMaxCandidates 单次融合允许扫描的文本条目上限（O(n²) 相似度比较的保护阈值）
	FuseMaxCandidates = 2000
	// FuseMaxContentChars 送入模型整合时单条内容的最大字符数
	FuseMaxContentChars = 1500
)

// 记忆融合策略
const (
	FuseStrategyAuto   = "auto"   // 短内容直接拼接，长内容交给模型整合
	FuseStrategyConcat = "concat" // 总是拼接去重
	FuseStrategyModel  = "model"  // 总是调用模型整合
)

// 图片识别取向常量 — 决定图片标签生成的描述角度与方式
const (
	RecognitionAuto       = "auto"       // 自动处理（默认）：综合性系统描述，由多模态模型自主决定重点
	RecognitionEmotion    = "emotion"    // 情绪表达：专注识别图片所表达的情绪
	RecognitionText       = "text"       // 文本内容：专注识别图片中的文字信息
	RecognitionColor      = "color"      // 色彩风格：专注分析主要配色、次要配色及点缀色
	RecognitionAppearance = "appearance" // 衣着发型：着重人物衣着、发型、身材、发色及瞳色
	RecognitionSpecies    = "species"    // 物种识别：着重事物种类与关键识别特征
	RecognitionPosture    = "posture"    // 姿态动作：重点表达肢体动作及人物表情
	RecognitionCustom     = "custom"     // 自定义：以用户文本作为识别取向参考
)

// 记忆库错误语义
var (
	// ErrNotFound 指定 ID 的记忆条目不存在
	ErrNotFound = errors.New("记忆条目不存在")

	// Types 全部合法记忆类型（顺序固定，便于错误提示与遍历）
	Types = []string{TypeText, TypeImage, TypeAudio}

	// DefaultCollections 默认分组集合名 — 覆盖聊天记忆可能对应的方方面面。
	//
	// 集合不再承担“查询表”职责，只是内容归组：启动时确保这些组存在并逐一用集合名
	// 生成嵌入向量；入库时把内容向量与全部集合名向量比较，归入最相关的一组。
	DefaultCollections = []string{
		// 对话与人物
		"日常闲聊",
		"人物信息",
		"关系互动",
		"情感心情",
		"兴趣爱好",
		"偏好习惯",
		"观点态度",
		"动作行为",
		// 经历与事务
		"事件经历",
		"计划日程",
		"工作学习",
		"知识见闻",
		"地点场景",
		// 媒体分组（按查询类型取用，集合本身只是语义分组）
		"表情包",
		"图片素材",
		"截图",
		"绘画作品",
		"语音消息",
		"音乐音频",
		"环境声音",
	}
)

// globalStore 全局记忆库实例（由 Init 创建，供包级 API 复用）
var globalStore *Store
