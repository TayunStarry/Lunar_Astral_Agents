package memory

import (
	"net/http"
	"sync"
	"time"
)

// =============================================================================
// 记忆库类型定义 — 文本/图片/音频混合存储，仅内容嵌入向量
// 核心约定：
//   - 文本与媒体记忆混合存储在同一个集合，不再区分"文本集合"/"图片集合"
//   - Document 统一以 content（文本）与 url（媒体）字段区分记忆类型
//   - 集合是内容按语义自动归入的分组，本身带一份集合名嵌入向量
// =============================================================================

// Entry 记忆库查询结果（含相似度分数）
// Type 为记忆类型（text/image/audio）；Collection 为内容自动归入的分组集合名。
// 调用方依据 type 取用对应内容，不再按集合名查询。
type Entry struct {
	ID         string  `json:"id"`                   // 文档 ID
	Type       string  `json:"type"`                 // 记忆类型：text / image / audio
	Role       string  `json:"role"`                 // 消息角色，图片记忆为 "image"
	Content    string  `json:"content"`              // 文本内容，文本记忆使用；图片记忆为空
	URL        string  `json:"url,omitempty"`        // 媒体读取地址（multimedia|audios/<集合名> 下的文件）
	Base64     string  `json:"base64,omitempty"`     // 旧版图片内联数据（仅历史数据兼容）
	Collection string  `json:"collection,omitempty"` // 内容所属分组集合名
	Source     string  `json:"source,omitempty"`     // 写入来源标记（插件 / 内部调用；普通对话为空）
	Similarity float32 `json:"similarity"`           // 内容向量余弦相似度；混合检索时为融合后的综合相关度
	Timestamp  int64   `json:"timestamp,omitempty"`  // 入库时间 Unix 秒级时间戳，旧数据无此字段
	// VectorScore 向量通道归一化得分（0~1，仅混合检索填写）
	VectorScore float32 `json:"vector_score,omitempty"`
	// KeywordScore 关键词通道命中强度（0~1，仅混合检索填写）
	KeywordScore float32 `json:"keyword_score,omitempty"`
	// MatchedBy 命中来源：vector / keyword / both（仅混合检索填写）
	MatchedBy string `json:"matched_by,omitempty"`
	// Pending 待处理（待合并）标记：写入时命中高度相似条目，等待整理合并。
	// 检索默认不返回待处理条目，只对整理可见。
	Pending bool `json:"pending,omitempty"`
	// PendingPartner 形成待合并组时记录的高度相似既有条目 ID
	PendingPartner string `json:"pending_partner,omitempty"`
	// Namespace 内部检索分区：private:<会话键> / group:<会话键> / shared。
	// 仅用于配额分配、去重与隐私边界，任何输出路径都不得渲染；旧数据为空表示未分区。
	Namespace string `json:"-"`
	// Channel 分区通道：private / group；旧数据为空。
	Channel string `json:"-"`
	// SpeakerKey 发言者内部键（用户 ID，无 ID 时为昵称哈希），永不渲染。
	SpeakerKey string `json:"-"`
}

// DocumentEntry 文档条目 — 用于前端分页列表（不含嵌入向量，避免传输开销）
type DocumentEntry struct {
	ID         string `json:"id"`                   // 文档条目 ID
	Type       string `json:"type"`                 // 记忆类型：text / image / audio
	Role       string `json:"role"`                 // 文档条目角色，"image" 表示图片记忆
	Content    string `json:"content"`              // 文本内容，文本记忆使用；图片记忆为空
	URL        string `json:"url,omitempty"`        // 媒体读取地址
	Base64     string `json:"base64,omitempty"`     // 旧版图片内联数据（仅历史数据兼容）
	Collection string `json:"collection,omitempty"` // 所属分组集合名
	Source     string `json:"source,omitempty"`     // 写入来源标记
	Timestamp  int64  `json:"timestamp,omitempty"`  // 入库时间 Unix 秒级时间戳，旧数据无此字段
	// Pending 待处理（待合并）标记；PendingPartner 待合并组中的高度相似既有条目 ID
	Pending        bool   `json:"pending,omitempty"`
	PendingPartner string `json:"pending_partner,omitempty"`
}

// QueryOptions 记忆向量检索选项
type QueryOptions struct {
	// ForceGlobal 强制全局检索：跳过「簇质心路由」，直接扫描全库（总库 + 全部正式簇）。
	//
	// 默认 false（快速检索）：先算查询与各正式簇质心的余弦，按 ClusterCentroidGate
	// 过滤后取前 QueryTopClusters 个簇，只在簇内成员里取 topK；结果不足时回落总库
	// （含未被路由命中的簇）补齐。需要“确保不漏、可接受全扫成本”时显式打开本项。
	ForceGlobal bool
}

// QueryStats 单次向量检索的路由观测数据（用于性能验证、日志与测试断言）。
//
// DocsCompared 是"本次实际计算了多少次条目余弦"，是衡量查询加速的唯一口径：
// 全库扫描时等于库内同类型带向量条目数；簇路由命中时只统计簇内成员 + 回落条目。
type QueryStats struct {
	ClustersTotal    int  `json:"clusters_total"`    // 索引中的正式簇总数
	ClustersCompared int  `json:"clusters_compared"` // 实际计算了质心余弦的簇数（维度不符的不计）
	ClustersHit      int  `json:"clusters_hit"`      // 通过质心门限、进入簇内检索的簇数（最多 QueryTopClusters）
	ClusterDocs      int  `json:"cluster_docs"`      // 簇内实际比较的成员条目数
	FallbackDocs     int  `json:"fallback_docs"`     // 回落阶段实际比较的条目数
	DocsCompared     int  `json:"docs_compared"`     // 实际比较条目总数（ClusterDocs + FallbackDocs）
	UsedFallback     bool `json:"used_fallback"`     // 是否发生回落（无簇命中或结果不足 topK）
	ForceGlobal      bool `json:"force_global"`      // 是否走了强制全局全扫
}

// HybridQueryOptions 混合检索（向量 + 关键词）选项
type HybridQueryOptions struct {
	// ForceGlobal 向量通道强制扫描全部集合（默认 false 走快速路由）
	ForceGlobal bool
	// VectorWeight 向量通道权重；与 KeywordWeight 同时为 0 时使用默认值，
	// 否则按两者之和归一化（允许只给其中一路非零权重）
	VectorWeight float32
	// KeywordWeight 关键词通道权重（归一化规则同上）
	KeywordWeight float32
	// CandidateLimit 每路召回上限（<=0 时按 topK 自适应：topK×3，夹在 20~60）
	CandidateLimit int
	// Regex 关键词通道按正则匹配（queries 以空格拼接后编译，忽略大小写）
	Regex bool
}

// hybridCandidate 混合检索融合过程中的单条候选（按记忆 ID 去重）
//
// 同一条记忆可能同时被向量通道与关键词通道命中，两路只保留一条：
// 向量分与关键词分各自取最大值，最终以融合分参与排序。
type hybridCandidate struct {
	entry        Entry   // 记忆条目（正文优先取更完整的一份）
	vectorScore  float32 // 归一化后的向量分（0~1）
	keywordScore float32 // 关键词命中强度（0~1）
	vectorHit    bool    // 是否被向量通道命中
	keywordHit   bool    // 是否被关键词通道命中
}

// =============================================================================
// 关键词倒排索引类型 — token → 条目 ID 倒排表，独立召回通道
// =============================================================================

// KeywordIndexStats 倒排索引体积与规模观测数据（用于容量评估与测试断言）。
type KeywordIndexStats struct {
	Docs           int   `json:"docs"`            // 索引内条目数（含媒体条目，媒体通常无词元）
	Tokens         int   `json:"tokens"`          // 倒排词表大小（不同 token 数）
	Postings       int   `json:"postings"`        // 倒排记录总数（token-条目 对数）
	FileBytes      int64 `json:"file_bytes"`      // 落盘文件字节数（gzip）；未落盘为 0
	EstimatedBytes int64 `json:"estimated_bytes"` // 内存体积估算（postings 按固定字节 + 词表开销）
}

// keywordIndexFile 倒排索引持久化快照（gzip JSON）。
// 文档顺序即文档序号（postings 中引用）；Tokens 为词表，Postings 扁平存放 [序号, 词频] 对。
type keywordIndexFile struct {
	Version     int                  `json:"version"`
	GeneratedAt int64                `json:"generated_at,omitempty"`
	Docs        []keywordDocRecord   `json:"docs"`
	Tokens      []keywordTokenRecord `json:"tokens"`
}

// keywordDocRecord 倒排索引中的条目元数据（不存正文，正文仍以分片文件为准）。
type keywordDocRecord struct {
	ID        string `json:"id"`
	Type      string `json:"type,omitempty"`
	Timestamp int64  `json:"ts,omitempty"`
	Pending   bool   `json:"pending,omitempty"`
	Hash      uint32 `json:"hash,omitempty"` // 正文哈希，用于启动时快速对账
}

// keywordTokenRecord 词元倒排表；Postings 按 [文档序号, 词频, 文档序号, 词频, ...] 扁平存储。
type keywordTokenRecord struct {
	Token    string   `json:"t"`
	Postings []uint32 `json:"p"`
}

// keywordIndex 内存倒排索引。由 Store.keywordMu 保护，内部方法不自行加锁。
type keywordIndex struct {
	docs   map[string]*keywordDoc      // 条目 ID → 条目元数据
	ords   []*keywordDoc               // 序号 → 条目元数据（删除后留 nil 空洞）
	tokens map[string][]keywordPosting // token → 倒排记录
	free   []uint32                    // 可复用的序号空洞
}

// keywordDoc 倒排索引中的单条记忆元数据。
type keywordDoc struct {
	ID         string
	Type       string
	Timestamp  int64
	Pending    bool
	Hash       uint32
	Collection string            // 运行时字段：当前所在集合名，不持久化，启动/搬迁时刷新
	Ord        uint32            // 文档序号
	Terms      map[string]uint16 // 该条目的词元与词频，用于删除/更新时精确撤销倒排
}

// keywordPosting 一条倒排记录。
type keywordPosting struct {
	Ord uint32 // 文档序号
	TF  uint16 // 词频（仅存储，覆盖率打分不使用命中次数）
}

// keywordIndexSource 重建索引时的条目来源快照（文档 + 所在集合）。
type keywordIndexSource struct {
	Doc        Document
	Collection string
}

// keywordRecallStats 单次关键词索引召回的观测数据。
type keywordRecallStats struct {
	UsedIndex       bool // 是否走倒排索引
	QueryTokens     int  // 生效查询词元数
	PostingsScanned int  // 扫描的倒排记录数（“比较条目数”口径）
	Candidates      int  // 去重后的候选条目数
	DocsScored      int  // 通过类型/待处理过滤后实际打分的条目数
	Hits            int  // 最终返回条目数
}

// keywordHitCandidate 倒排召回后的候选（仅保留构建 Entry 所需的最小字段）。
type keywordHitCandidate struct {
	ID         string
	Collection string
	Timestamp  int64
	Score      float32
}

// FuseOptions 权重融合参数（仅作用于文本记忆）
type FuseOptions struct {
	// Threshold 同簇相似度阈值（余弦，0~1）；<=0 或低于下限时用默认值
	Threshold float32
	// Strategy 合并策略：auto（短拼接/长模型）、concat（拼接去重）、model（模型整合）
	Strategy string
	// Role 合并后条目的角色；为空时取簇内出现最多的角色
	Role string
}

// FuseGroup 一组被合并的条目
type FuseGroup struct {
	// IDs 被合并（并删除）的原条目 ID
	IDs []string `json:"ids"`
	// Similarity 簇内最低相似度（相对簇代表条目）
	Similarity float32 `json:"similarity"`
	// Content 合并后的正文
	Content string `json:"content"`
	// Role 合并后条目角色
	Role string `json:"role"`
	// NewID 新建的整合条目 ID
	NewID string `json:"new_id"`
	// Strategy 实际使用的合并方式：concat / model
	Strategy string `json:"strategy"`
}

// FuseResult 权重融合结果
type FuseResult struct {
	Collection     string      `json:"collection"`
	CandidateCount int         `json:"candidate_count"` // 参与扫描的文本条目数
	GroupCount     int         `json:"group_count"`     // 实际合并的簇数
	CreatedCount   int         `json:"created_count"`   // 新建整合条目数
	RemovedCount   int         `json:"removed_count"`   // 删除的原条目数
	SkippedMedia   int         `json:"skipped_media"`   // 因非文本/无正文被跳过的条目数
	Threshold      float32     `json:"threshold"`
	Groups         []FuseGroup `json:"groups"`
}

// TextCluster 一组语义相近的文本条目（只读规划结果，不做任何写入）。
// IDs/Contents/Roles 按入库时间升序对齐；Role 取簇内出现次数最多的角色。
type TextCluster struct {
	IDs             []string `json:"ids"`
	Contents        []string `json:"contents"`
	Roles           []string `json:"roles,omitempty"`
	Role            string   `json:"role,omitempty"`
	MinSimilarity   float32  `json:"min_similarity"`
	OldestTimestamp int64    `json:"oldest_timestamp,omitempty"`
	NewestTimestamp int64    `json:"newest_timestamp,omitempty"`
}

// ReplaceOptions 批量替换文本记忆的参数（记忆整理落库使用）
type ReplaceOptions struct {
	// Role 新条目角色；为空时按 assistant 处理
	Role string
	// Source 新条目来源标记；为空时按 consolidation 处理
	Source string
}

// ReplaceResult 批量替换结果
type ReplaceResult struct {
	// CreatedIDs 本次新建的条目 ID（回滚时只删除这些）
	CreatedIDs []string `json:"created_ids,omitempty"`
	// ReusedIDs 命中「完全同文」的既有条目 ID（不重复写入）
	ReusedIDs []string `json:"reused_ids,omitempty"`
	// DeletedIDs 本次成功删除的旧条目 ID
	DeletedIDs []string `json:"deleted_ids,omitempty"`
}

// docSnapshot 替换前对旧条目的完整快照（含所属集合，用于失败回滚）
type docSnapshot struct {
	Doc        Document
	Collection string
}

// Document 统一文档结构（文本 / 图片 / 音频记忆共用，混合存储在同一个集合）
// content 与 url 二选一：文本记忆 content 存储文本内容、url 为空；
// 媒体记忆 url 指向 multimedia|audios/<集合名>/ 下的实体文件、content 通常为空。
// base64 仅用于兼容历史数据，新写入不再产生。Embedding 为内容嵌入向量：
// 文本记忆为正文（或预设标签拼接文本）的向量，媒体记忆为标签/描述文本的向量。
type Document struct {
	ID        string `json:"id"`                  // 文档 UUID v4
	Type      string `json:"type,omitempty"`      // 记忆类型：text / image / audio（旧数据为空，按字段推断）
	Role      string `json:"role,omitempty"`      // 消息角色，文本记忆使用
	Content   string `json:"content,omitempty"`   // 文本内容，文本记忆使用
	URL       string `json:"url,omitempty"`       // 媒体读取地址（multimedia|audios/<集合名> 下的文件）
	Base64    string `json:"base64,omitempty"`    // 旧版图片内联数据（仅历史数据兼容）
	Source    string `json:"source,omitempty"`    // 写入来源标记（插件 / 内部调用），仅作溯源信息
	Timestamp int64  `json:"timestamp,omitempty"` // 入库时间 Unix 秒级时间戳
	// Pending 待处理（待合并）标记：命中高度相似条目但保留新措辞，等待整理合并。
	// 旧库没有该字段时按 false 处理，无需迁移。
	Pending bool `json:"pending,omitempty"`
	// PendingPartner 待合并组中的高度相似既有条目 ID（仅 Pending 为 true 时有值）
	PendingPartner string `json:"pending_partner,omitempty"`
	// PendingAttempts 该条目在待合并状态下连续失败的次数；达到上限后解除待处理标记。
	PendingAttempts int `json:"pending_attempts,omitempty"`
	// PendingSince 条目首次进入待合并状态的时间（Unix 秒）；用于超时放行，避免无限隐身。
	PendingSince int64 `json:"pending_since,omitempty"`
	// Namespace 内部检索分区：private:<会话键> / group:<会话键> / shared。
	// 仅用于配额分配、去重与隐私边界，永不渲染；旧数据为空表示未分区。
	Namespace string `json:"namespace,omitempty"`
	// Channel 分区通道：private / group；旧数据为空。
	Channel string `json:"channel,omitempty"`
	// SpeakerKey 发言者内部键（用户 ID，无 ID 时为昵称哈希），仅用于每用户配额与去重，永不渲染。
	SpeakerKey string `json:"speaker_key,omitempty"`
	// LegacyPrefix 迁移期保存的原始前缀，仅供还原与回滚；不参与渲染与检索排序，迁移完成后为纯内部记账字段。
	LegacyPrefix string `json:"legacy_prefix,omitempty"`
	// Embedding 文档内容嵌入向量，json:"-" 使其不随文档持久化，
	// 单独存储于 embeddings_NNNN.json 切片文件（见 embeddingRecord）。
	// 带 tags 写入时该字段是标签向量（检索用），正文向量见 ContentEmbedding。
	Embedding []float32 `json:"-"`
	// ContentEmbedding 正文嵌入向量；仅带 tags 写入的文本记忆会单独保存。
	// 判重、聚类与整理结果校验一律以正文向量为准，避免标签向量污染。
	ContentEmbedding []float32 `json:"-"`
}

// embeddingRecord 嵌入向量切片条目 — 与文档切片一一对应（按相同索引对齐）
// 由于 Document.Embedding 为 json:"-"，向量单独以 ID 关联持久化，
// 避免文档切片文件体积过大。
type embeddingRecord struct {
	ID        string    `json:"id"`        // 对应文档的 UUID
	Embedding []float32 `json:"embedding"` // 检索用嵌入向量（有 tags 时为标签向量）
	// ContentEmbedding 正文嵌入向量；无标签写入时为空（正文向量即 Embedding）
	ContentEmbedding []float32 `json:"content_embedding,omitempty"`
}

// PendingFailureResult 一次整理失败后的待处理状态变化。
type PendingFailureResult struct {
	// Released 达到重试上限或隐身超时，已解除待处理标记并恢复检索可见的条目
	Released []string
	// Retained 仍保持待处理状态、等待下一轮重试的条目
	Retained []string
}

// collectionMeta 集合元数据，持久化到 metadata.json
type collectionMeta struct {
	EmbeddingDimension   int       `json:"embedding_dimension"`            // 锁定的向量维度
	NameEmbedding        []float32 `json:"name_embedding,omitempty"`       // 集合名嵌入向量（内容归组路由用）
	NameEmbeddingModel   string    `json:"name_embedding_model,omitempty"` // 集合名嵌入所用模型
	Version              int       `json:"version"`                        // 数据格式版本号（v5 = 5）
	DocumentsChunkCount  int       `json:"documents_chunk_count"`          // 文档分块数
	EmbeddingsChunkCount int       `json:"embeddings_chunk_count"`         // 嵌入向量分块数
}

// Collection 物理存储单元 — 总库或正式簇。
//
// 语义（v6）：总库是主体（Name = "store-all"），正式簇是加速结构（Name = "cluster-xxx"）。
// 所有条目默认进总库；只有高相似组 ≥ 16 条才升级为正式簇。
// 总库条目**不出现在** ClusterIndex.Assignments 里（隐含归属）。
//
// 存储布局：
//
//	总库：<baseDir>/store-all/{metadata.json, documents_*.json, embeddings_*.json}
//	簇  ：<baseDir>/c-<sha256(簇 ID)[:8]>/{metadata.json, documents_*.json, embeddings_*.json}
type Collection struct {
	Name                 string         // ID：总库固定 "store-all"；簇为 "cluster-xxx"
	Kind                 string         // "store" 或 "cluster"
	Dimension            int            // 锁定的向量维度
	NameEmbedding        []float32      // 旧版集合名向量（仅历史兼容，不再参与归组）
	NameEmbeddingModel   string         // 旧版集合名向量模型（仅历史兼容）
	Documents            []Document     // 该单元条目列表（文本/图片/音频混合，含内存嵌入向量）
	mu                   sync.RWMutex   // 数据读写锁
	collDir              string         // 目录绝对路径（总库 = store-all；簇 = c-<hash8>）
	metaPath             string         // metadata.json 路径
	documentsChunkCount  int            // 文档分块数
	embeddingsChunkCount int            // 嵌入向量分块数
	lastFileModTime      time.Time      // metadata.json 最近加载时间，用于跨进程一致性检测
	positions            map[string]int // 条目 ID → Documents 下标（倒排索引召回后 O(1) 定位正文）
}

// Store 记忆库实例（总库 + 正式簇 双轨架构）
// 职责：嵌入服务连接、LLM 标签生成、簇管理、记忆 CRUD、维度锁定、持久化
// 存储布局：
//
//	<baseDir>/store-all/{metadata.json, documents_*.json, embeddings_*.json}  总库
//	<baseDir>/c-<hash8>/{metadata.json, documents_*.json, embeddings_*.json}  正式簇
//	<baseDir>/clusters.json  聚类索引（唯一的分组依据：条目 ID → 簇 ID；总库条目**不在** Assignments 里）
type Store struct {
	initMu        sync.Mutex             // 实例初始化互斥锁（启动/懒加载并发保护）
	initialized   bool                   // 记忆库实例是否初始化完成
	httpClient    *http.Client           // HTTP 客户端（嵌入 + LLM 共享）
	llmMu         sync.Mutex             // LLM 调用互斥锁（严格单线程）
	baseDir       string                 // 记忆存储根目录绝对路径
	loadErr       error                  // 启动加载首个失败原因（nil 表示加载正常），供 ListWithError 表达「读不到数据」
	collections   map[string]*Collection // 簇 ID → 该簇的物理存储单元
	collectionsMu sync.RWMutex           // collections map 读写锁
	clusterMu     sync.RWMutex           // 聚类索引读写锁
	clusterIndex  *ClusterIndex          // 向量空间聚类索引（nil 表示尚未加载/迁移）
	clusterMod    time.Time              // clusters.json 最近加载时间，用于外部变更检测
	keywordMu     sync.RWMutex           // 倒排索引读写锁
	keywordIndex  *keywordIndex          // 关键词倒排索引（nil 表示尚未建立，查询时懒加载/重建）
}

// ClusterIndex 聚类索引 — 总库为主，正式簇为加速结构。
// 持久化到 <baseDir>/clusters.json。
//
// Assignments 只包含**正式簇**的成员；总库条目不在此表里（隐含归属）。
// 不在 Assignments 里的条目就是总库条目。
type ClusterIndex struct {
	Version      int               `json:"version"`   // 索引版本（v6 = 2）
	Threshold    float32           `json:"threshold"` // 入簇阈值（簇内成员与质心余弦）
	Model        string            `json:"model,omitempty"`
	Dimension    int               `json:"dimension"`
	StoreSize    int               `json:"store_size"`    // 总库条目数
	StoreVectors int               `json:"store_vectors"` // 总库带向量条目数
	Clusters     []ClusterNode     `json:"clusters"`      // 正式簇列表（不含总库）
	Assignments  map[string]string `json:"assignments"`   // 条目 ID → 正式簇 ID（总库条目不在此表）
	UpdatedAt    int64             `json:"updated_at"`
}

// ClusterNode 一个向量簇：质心、成员与规模。
type ClusterNode struct {
	ID        string    `json:"id"`                   // 簇 ID（重建时由簇内最早条目的 ID 派生，保证幂等）
	Centroid  []float32 `json:"centroid"`             // 归一化的簇质心
	DocIDs    []string  `json:"doc_ids"`              // 簇内条目 ID（按并入顺序）
	Size      int       `json:"size"`                 // 成员数量（与 DocIDs 长度对账）
	UpdatedAt int64     `json:"updated_at,omitempty"` // 最近更新时间
}

// ClusterMigrationReport 聚类迁移/重建的对账结果。
type ClusterMigrationReport struct {
	BackupDir     string `json:"backup_dir,omitempty"`
	DocCount      int    `json:"doc_count"`      // 全部条目数
	VectorCount   int    `json:"vector_count"`   // 带向量的条目数
	StoreSize     int    `json:"store_size"`     // 迁移后总库条目数
	ClusterCount  int    `json:"cluster_count"`  // 迁移后正式簇数量
	AssignedCount int    `json:"assigned_count"` // 正式簇成员总数
	FileCount     int    `json:"file_count"`     // 所有目录分片文件总数（总库 + 簇）
	MovedDocs     int    `json:"moved_docs"`     // 搬迁条目数（旧布局→新布局 或 簇↔总库）
	LegacyDirs    int    `json:"legacy_dirs"`    // 迁移后删除的旧布局目录数
	ReusedIndex   bool   `json:"reused_index"`   // 是否复用既有 clusters.json
}

// ClusterBackupManifest 迁移备份清单。
type ClusterBackupManifest struct {
	Timestamp       int64          `json:"timestamp"`
	HadClusterIndex bool           `json:"had_cluster_index"`
	DocCount        int            `json:"doc_count"`
	VectorCount     int            `json:"vector_count"`
	Dirs            map[string]int `json:"dirs,omitempty"`
	LegacyDirs      []string       `json:"legacy_dirs,omitempty"`
}

// diskLayout 磁盘扫描结果。
type diskLayout struct {
	Docs        map[string]Document // 全部条目（旧目录优先）
	DocDirs     map[string]string   // 条目 ID → 当前所在目录
	LegacyDirs  []string            // 旧布局目录（迁移后删除）
	StoreDir    string              // 总库目录路径（存在时）
	ClusterDirs []string            // 正式簇目录 c-*
	FileCount   int                 // 所有目录分片文件总数
}

// fuseCluster 一个相似簇：indices 指向候选切片，首元素为代表条目
type fuseCluster struct {
	indices       []int
	minSimilarity float32
}

// =============================================================================
// 嵌入 / LLM 请求响应类型（OpenAI 兼容）
// =============================================================================

// embeddingRequest 对应 OpenAI 兼容 /v1/embeddings 请求体
type embeddingRequest struct {
	Model string   `json:"model"` // 模型名，固定 [text_embedding]
	Input []string `json:"input"` // 待嵌入文本列表
}

// embeddingData 对应响应 data 数组单项
type embeddingData struct {
	Embedding []float32 `json:"embedding"` // 嵌入向量
}

// embeddingResponse 对应 OpenAI 兼容 /v1/embeddings 响应体
type embeddingResponse struct {
	Data  []embeddingData `json:"data"` // 嵌入向量列表，与 input 等长
	Error *struct {
		Message string `json:"message"` // 错误描述
	} `json:"error,omitempty"` // 错误响应载荷
}
