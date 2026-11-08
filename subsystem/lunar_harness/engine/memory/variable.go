package memory

import (
	"context"
	"errors"
	"time"
)

// =============================================================================
// 智能体装配钩子 — 模型调用统一由 agents 层实现，engine 不反向依赖 agents
// =============================================================================

// TagGenerator 标签生成智能体（实现于 agents/MemoryTagger）。
var TagGenerator func(ctx context.Context, content string, isImage bool, orientation, custom string, mediaRefs []string) ([]string, error)

// MemoryMergeFunc 记忆整合智能体（实现于 agents/MemoryFuser）。
var MemoryMergeFunc func(ctx context.Context, contents []string) (string, error)

// MediaSubdir 记忆库图片在 multimedia 下的专属子目录：
// 图片实体统一落在 multimedia/memory/<条目ID>/，与生成图、背景图、截图等隔离，便于清理。
const MediaSubdir = "memory"

// =============================================================================
// 磁盘布局常量 — 总库 + 正式簇 双轨
//
// 语义：总库是主体，簇是加速结构。所有条目默认进总库；
// 只有当总库中高相似条目攒到 ≥ ClusterUpgradeSize 条时才升级成正式簇。
// =============================================================================
const (
	// StoreCollectionID 总库的固定 ID（也是总库目录名）
	StoreCollectionID = "store-all"
	// ClusterDirPrefix 正式簇目录名前缀；完整名称为 c-<sha256(簇 ID)[:8]>
	ClusterDirPrefix = "c-"
	// ClusterDirHashLen 簇目录短标识的哈希长度（十六进制字符数）
	ClusterDirHashLen = 8
	// ClusterLayoutManifest 迁移备份目录内的布局清单文件名
	ClusterLayoutManifest = "manifest.json"
)

// v6 记忆库配置常量 — 总库 + 正式簇 双轨存储
const (
	CurrentVersion     = 6    // 数据格式版本号（v6: 总库 + 正式簇，簇需 ≥ 16 条）
	DocumentsChunkSize = 100  // 文档分块大小（条/块，各类型统一；100 条一块）
	DedupThreshold     = 0.98 // 判重阈值（余弦相似度）：≥ 0.98 算同一条记忆
	DocRankThreshold   = 0.30 // 文档内容余弦得分门槛（低于此值不返回）
)

// 阈值语义分开说明：
//   DedupThreshold    = 0.98  判重：两个条目几乎完全相同才算重复
//   ClusterAdmitThreshold = 0.90  入簇：簇内成员与质心相似度需 ≥ 0.90
//   ClusterUpgradeSize   = 16 升级：总库中高相似组 ≥ 16 条才升簇
//   ClusterDowngradeSize = 8  降级：簇内删除到 < 8 条 → 解散回总库
//   ClusterCentroidGate  = 0.70 查询时质心剪枝门限：余弦 < 0.70 跳过该簇
//   ClusterMemberGate    = 0.80 查询时簇内成员门限：余弦 < 0.80 跳过

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
	// （记忆整合的单条内容截断长度由 agents/MemoryFuser 负责）
)

// 记忆整理候选聚类参数（chain/life 与 Fuse 管理端点共用同一套聚类原语）
const (
	// ClusterDefaultThreshold 默认相似度阈值：同簇条目与该簇最早条目（代表）需达到此值
	ClusterDefaultThreshold float32 = 0.90
	// ClusterMinThreshold 允许的最小阈值，低于此值会被夹回默认值
	ClusterMinThreshold float32 = 0.50
	// ClusterVerifyThreshold 整理结果与来源记忆的双向相似度校验门槛
	ClusterVerifyThreshold float32 = 0.80
	// ClusterMaxItems 单个相似簇的条目上限，避免把一大片弱相关记忆串成一簇
	ClusterMaxItems = 8
	// ClusterMaxRunes 单个相似簇的正文总字符上限
	ClusterMaxRunes = 6000
	// fuseReplaceSource Fuse 管理端点整合后新条目的来源标记
	fuseReplaceSource = "fusion"
)

// 记忆融合策略
const (
	FuseStrategyAuto   = "auto"   // 短内容直接拼接，长内容交给模型整合
	FuseStrategyConcat = "concat" // 总是拼接去重
	FuseStrategyModel  = "model"  // 总是调用模型整合
)

// 混合检索（向量 + 关键词）融合参数
const (
	// HybridVectorWeight 默认向量通道权重（语义相近度）
	HybridVectorWeight float32 = 0.6
	// HybridKeywordWeight 默认关键词通道权重（字面命中强度）
	HybridKeywordWeight float32 = 0.4
	// HybridCandidateMultiplier 每路召回上限 = topK × 本倍数
	HybridCandidateMultiplier = 3
	// HybridMinCandidates 每路召回下限（短查询也能有足够候选）
	HybridMinCandidates = 20
	// HybridMaxCandidates 每路召回硬上限
	HybridMaxCandidates = 60
	// HybridKeywordOccurrenceCap 关键词命中次数计入强度的上限（超过按上限计）
	HybridKeywordOccurrenceCap = 3
	// HybridKeywordOccurrenceFloor 关键词命中强度下限：覆盖度 × 本值起步，命中越多越接近 1
	HybridKeywordOccurrenceFloor float32 = 0.6
	// HybridVectorTimeout 向量通道超时（嵌入服务不可用时尽快降级到关键词通道）
	HybridVectorTimeout = 8 * time.Second
)

// 关键词倒排索引参数 — 与向量通道共享召回预算，独立召回后统一融合
const (
	// KeywordIndexVersion 倒排索引版本号；版本不一致时启动阶段幂等重建
	KeywordIndexVersion = 1
	// KeywordIndexFile 倒排索引文件名（位于记忆库根目录，gzip 压缩的 JSON）
	KeywordIndexFile = "keyword_index.json.gz"
	// KeywordExactWeight 精确串（路径/错误码/编号等）词元权重倍数：可单独触发召回
	KeywordExactWeight float32 = 1.6
	// KeywordMaxQueryTokens 单次查询参与召回的词元上限（优先保留精确串与稀有词）
	KeywordMaxQueryTokens = 12
	// KeywordMinScore 关键词覆盖率得分下限：低于此值视为弱相关（如仅命中大停用词），不进入融合候选
	KeywordMinScore float32 = 0.05
	// KeywordMinASCIIWordLen ASCII 单词最短长度（单字符噪声大，不建索引）
	KeywordMinASCIIWordLen = 2
	// KeywordPostingBytesEstimate 单条倒排记录（文档序号+词频）的体积估算字节数
	KeywordPostingBytesEstimate = 8
)

// 向量空间聚类索引参数 — 总库为主，正式簇为加速
const (
	// ClusterIndexVersion 聚类索引版本
	ClusterIndexVersion = 2
	// ClusterIndexFile 聚类索引文件名（位于记忆库根目录）
	ClusterIndexFile = "clusters.json"
	// ClusterMigrateBackupDir 迁移备份目录名（位于记忆库根目录下）
	ClusterMigrateBackupDir = "migration_backup"

	// ===== 入簇/升级/降级阈值（语义分开）=====
	// ClusterAdmitThreshold 入簇阈值：簇内成员与质心余弦 ≥ 本值才能留在簇（保证簇内紧凑）
	ClusterAdmitThreshold float32 = 0.90
	// ClusterUpgradeSize 升级门槛：总库中高相似组 ≥ 本条才升为正式簇
	ClusterUpgradeSize = 16
	// ClusterDowngradeSize 降级门槛：簇内删除到 < 本条 → 解散回总库
	ClusterDowngradeSize = 8

	// ===== 簇内聚类辅助参数（重建时使用，非最终对外语义）=====
	// ClusterMergeThreshold 重建时合并质心余弦 ≥ 本值的簇
	ClusterMergeThreshold float32 = 0.95
	// ClusterSplitThreshold 簇内平均紧凑度低于本值时触发分裂
	ClusterSplitThreshold float32 = 0.60
	// ClusterMaxMembers 单簇成员上限（超过触发分裂）
	ClusterMaxMembers = 64

	// ===== 查询路由参数（剪枝门限，越宽松剪枝越多但可能漏召回）=====
	// ClusterCentroidGate 查询时质心剪枝门限：余弦 < 本值跳过该簇
	ClusterCentroidGate float32 = 0.70
	// ClusterMemberGate 查询时簇内成员门限：余弦 < 本值跳过
	ClusterMemberGate float32 = 0.80
	// QueryTopClusters 查询时质心排序后前 N 个簇参与簇内检索（其余簇走总库回落）
	QueryTopClusters = 5
)

// 混合检索命中的来源标记（Entry.MatchedBy）
const (
	// MatchedByVector 仅向量通道命中
	MatchedByVector = "vector"
	// MatchedByKeyword 仅关键词通道命中
	MatchedByKeyword = "keyword"
	// MatchedByBoth 两个通道同时命中
	MatchedByBoth = "both"
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
)

// 待处理（待合并）队列参数
var (
	// PendingMaxEntries 待处理条目的写入上限（全局）。达到上限后新条目仍然照常写入，
	// 只是不再打待处理标记：队列不会无限膨胀，任何情况下都不会先丢数据。
	PendingMaxEntries = 200
)

// globalStore 全局记忆库实例（由 Init 创建，供包级 API 复用）
var globalStore *Store
