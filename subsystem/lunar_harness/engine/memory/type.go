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
	Similarity float32 `json:"similarity"`           // 内容向量余弦相似度
	Timestamp  int64   `json:"timestamp,omitempty"`  // 入库时间 Unix 秒级时间戳，旧数据无此字段
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
}

// QueryOptions 记忆向量检索选项
type QueryOptions struct {
	// ForceGlobal 强制全局检索：跳过「集合名向量」快速路由，扫描全部集合。
	//
	// 默认 false（快速检索）：先查与查询含义最接近的集合；命中则再查第 2~5 近的
	// 集合，首集合未命中则回退扫描其余全部集合。集合越多，快速检索省下的全量
	// 向量比较越多；需要“确保不漏”时（如表情包等稀疏媒体）显式打开本项。
	ForceGlobal bool
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
	// Embedding 文档内容嵌入向量，json:"-" 使其不随文档持久化，
	// 单独存储于 embeddings_NNNN.json 切片文件（见 embeddingRecord）
	Embedding []float32 `json:"-"`
}

// embeddingRecord 嵌入向量切片条目 — 与文档切片一一对应（按相同索引对齐）
// 由于 Document.Embedding 为 json:"-"，向量单独以 ID 关联持久化，
// 避免文档切片文件体积过大。
type embeddingRecord struct {
	ID        string    `json:"id"`        // 对应文档的 UUID
	Embedding []float32 `json:"embedding"` // 文档内容嵌入向量
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

// Collection 单个记忆集合 — 内容按语义自动归入的分组
// 集合本身也有一份「集合名嵌入向量」：入库时把内容的嵌入向量与所有集合名向量比较，
// 归入最相关的一组；查询则在全部集合中检索，不再由调用方指定集合。
// 存储布局：
//
//	<collDir>/metadata.json
//	<collDir>/documents_NNNN.json   (文本/图片/音频统一分块存储，不含向量，100 条/块)
//	<collDir>/embeddings_NNNN.json  (内容嵌入向量分块存储，100 条/块)
type Collection struct {
	Name                 string       // 集合名
	Dimension            int          // 锁定的向量维度
	NameEmbedding        []float32    // 集合名嵌入向量
	NameEmbeddingModel   string       // 集合名嵌入所用模型
	Documents            []Document   // 统一文档列表（文本/图片/音频混合，含内存嵌入向量）
	mu                   sync.RWMutex // 数据读写锁
	collDir              string       // 集合目录绝对路径
	metaPath             string       // metadata.json 路径
	documentsChunkCount  int          // 文档分块数
	embeddingsChunkCount int          // 嵌入向量分块数
	lastFileModTime      time.Time    // metadata.json 最近加载时间，用于跨进程一致性检测
}

// Store 记忆库实例（多集合分组架构）
// 职责：嵌入服务连接、LLM 标签生成、集合管理、记忆 CRUD、维度锁定、持久化
// 模型配置与智能体共用 agent.embedding_* / agent.multimodal_*（不再有独立记忆库模型配置）
// 存储布局：<baseDir>/<collectionName>/{metadata.json, documents_*.json, embeddings_*.json}
type Store struct {
	initMu        sync.Mutex             // 实例初始化互斥锁（启动/懒加载并发保护）
	initialized   bool                   // 记忆库实例是否初始化完成
	httpClient    *http.Client           // HTTP 客户端（嵌入 + LLM 共享）
	llmMu         sync.Mutex             // LLM 调用互斥锁（严格单线程）
	baseDir       string                 // 记忆存储根目录绝对路径
	collections   map[string]*Collection // 集合名 → 集合实例
	collectionsMu sync.RWMutex           // collections map 读写锁
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

// chatRequest OpenAI 兼容 /v1/chat/completions 请求体
type chatRequest struct {
	Model              string         `json:"model"`
	Messages           []chatMessage  `json:"messages"`
	MaxTokens          int            `json:"max_tokens,omitempty"`
	Temperature        float32        `json:"temperature,omitempty"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

// chatMessage 聊天消息
type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string 或 []chatContentPart
}

// chatContentPart 多模态消息内容部分
type chatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片 URL 引用
type imageURL struct {
	URL string `json:"url"`
}

// chatResponse OpenAI 兼容 /v1/chat/completions 响应体
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// chatChoice 聊天响应选择
type chatChoice struct {
	Message chatMessageResp `json:"message"`
}

// chatMessageResp 聊天响应消息
type chatMessageResp struct {
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}
