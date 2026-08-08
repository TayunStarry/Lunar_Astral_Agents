package module

import (
	"database/sql"
	"net/http"
	"sync"
	"time"
)

// FileInfo 文件信息结构体，用于存储文件和目录的相关信息
type FileInfo struct {
	Name         string    `json:"name"`         // 文件名或目录名
	Size         int64     `json:"size"`         // 文件大小
	IsDir        bool      `json:"isDir"`        // 是否为目录
	LastModified time.Time `json:"lastModified"` // 最后修改时间
	Path         string    `json:"path"`         // 相对于配置目录的相对路径
}

// OperationResult 单个操作结果
type OperationResult struct {
	Success      bool             `json:"success"`                  // 是否成功执行
	Error        string           `json:"error,omitempty"`          // 错误信息
	Operation    string           `json:"operation"`                // 操作类型
	Rows         []map[string]any `json:"rows,omitempty"`           // 受影响的行数据
	AffectedRows int64            `json:"affected_rows,omitempty"`  // 受影响的行数
	LastInsertID int64            `json:"last_insert_id,omitempty"` // 最后插入的行ID
	Table        string           `json:"table,omitempty"`          // 受影响的表名
	Structure    []map[string]any `json:"structure,omitempty"`      // 受影响的表结构定义
	Tables       []string         `json:"tables,omitempty"`         // 受影响的表名列表
	Count        int64            `json:"count,omitempty"`          // 受影响的行数
}

// BatchResult 批量操作结果
type BatchResult struct {
	Success    bool              `json:"success"`         // 是否所有成功执行
	Error      string            `json:"error,omitempty"` // 错误信息
	Results    []OperationResult `json:"results"`         // 操作结果列表
	TotalTime  int64             `json:"total_time_ms"`   // 总耗时（毫秒）
	Operations int               `json:"operations"`      // 操作次数
}

// ColumnDefinition 列定义
type ColumnDefinition struct {
	Name          string      `json:"name"`                     // 列名
	Type          string      `json:"type"`                     // 列类型
	PrimaryKey    bool        `json:"primary_key,omitempty"`    // 是否主键
	AutoIncrement bool        `json:"auto_increment,omitempty"` // 是否自动递增
	NotNull       bool        `json:"not_null,omitempty"`       // 是否非空
	Unique        bool        `json:"unique,omitempty"`         // 是否唯一
	Default       interface{} `json:"default,omitempty"`        // 默认值
}

// TableDefinition 表定义
type TableDefinition struct {
	Columns []ColumnDefinition `json:"columns"`           // 列定义列表
	Indexes []IndexDefinition  `json:"indexes,omitempty"` // 索引定义列表
}

// IndexDefinition 索引定义
type IndexDefinition struct {
	Name    string   `json:"name"`             // 索引名称
	Columns []string `json:"columns"`          // 索引列名列表
	Unique  bool     `json:"unique,omitempty"` // 是否唯一
}

// KnowledgeRequest 知识库请求
type KnowledgeRequest struct {
	Operations  []interface{} `json:"operations"`            // 知识库操作列表 ，每个元素可以是DataOperation, TableOperation, InfoOperation
	Transaction bool          `json:"transaction,omitempty"` // 是否开启事务，默认 false
}

// =============================================================================
// v2 记忆库类型定义 — 标签向量中介检索架构
// =============================================================================

// memoryMessage 记忆库查询返回的兼容消息结构（仅用于 MemoryQueryMessages 的 JSON 编码）
type memoryMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 消息内容
}

// MemoryQueryResult 记忆库查询结果（含相似度分数）
// v2: Similarity 字段表示标签向量匹配频次得分（C_i / N），范围 [0, 1]
type MemoryQueryResult struct {
	ID         string  `json:"id"`              // 文档 ID
	Role       string  `json:"role"`            // 消息角色，image 文档为 "image"
	Content    string  `json:"content"`         // 消息内容，image 文档为空
	Image      string  `json:"image,omitempty"` // 图片 base64 数据，仅 image 文档
	Similarity float32 `json:"similarity"`      // 标签匹配频次得分
}

// DocumentEntry 文档条目 — 用于前端分页列表（不含嵌入向量，避免传输开销）
type DocumentEntry struct {
	ID      string `json:"id"`              // 文档条目 ID
	Role    string `json:"role"`            // 文档条目角色，"image" 表示图片文档
	Content string `json:"content"`         // 文档条目内容，image 文档为空
	Image   string `json:"image,omitempty"` // 图片 base64 数据，仅 image 文档
}

// Document 统一文档结构（text 和 image 共用）
// text 文档：ID + Role + Content
// image 文档：ID + Image
type Document struct {
	ID      string `json:"id"`                // 文档 UUID v4
	Role    string `json:"role,omitempty"`    // 消息角色，text 文档使用
	Content string `json:"content,omitempty"` // 文本内容，text 文档使用
	Image   string `json:"image,omitempty"`   // 图片 base64 数据，image 文档使用
}

// TagVector 标签向量条目 — 标签文本的嵌入向量及其关联的原始文档 UUID 数组
// 标签向量在集合内全局共享，多个文档可关联同一标签向量
type TagVector struct {
	Tag       string    `json:"tag"`       // 标签文本（保留，便于调试和可解释性）
	Embedding []float32 `json:"embedding"` // 标签文本的嵌入向量
	UUIDs     []string  `json:"uuid"`      // 关联的原始文档 UUID 数组
}

// collectionMeta v2 集合元数据，持久化到 metadata.json
type collectionMeta struct {
	EmbeddingModel      string `json:"embedding_model"`       // 锁定的嵌入模型名
	EmbeddingDimension  int    `json:"embedding_dimension"`   // 锁定的向量维度
	ChunkCount          int    `json:"chunk_count,omitempty"` // 已废弃，保留兼容
	MultimodalModel     string `json:"multimodal_model"`      // 标签生成多模态模型名
	Type                string `json:"type"`                  // 集合类型："text" 或 "image"
	Version             int    `json:"version"`               // 数据格式版本号（v2 = 2）
	DocumentsChunkCount int    `json:"documents_chunk_count"` // text 文档分块数
	ImagesChunkCount    int    `json:"images_chunk_count"`    // image 文档分块数
	TagsChunkCount      int    `json:"tags_chunk_count"`      // 标签向量分块数
}

// Collection 单个记忆集合 — v2 标签向量中介检索架构
// 文本与图片文档统一为 Document 列表，标签向量独立存储
// 存储布局：
//
//	<collDir>/metadata.json
//	<collDir>/documents_NNNN.json  (text 文档，500 条/块)
//	<collDir>/images_NNNN.json     (image 文档，20 条/块)
//	<collDir>/tags_NNNN.json       (标签向量，100 条/块)
type Collection struct {
	Name                string       // 集合名
	Model               string       // 锁定的嵌入模型名
	Dimension           int          // 锁定的向量维度
	CollectionType      string       // 集合类型："text" 或 "image"
	MultimodalModel     string       // 标签生成多模态模型名（来自全局配置）
	Documents           []Document   // 统一文档列表（text 或 image）
	TagVectors          []TagVector  // 标签向量列表（常驻内存）
	mu                  sync.RWMutex // 数据读写锁
	collDir             string       // 集合目录绝对路径
	metaPath            string       // metadata.json 路径
	documentsChunkCount int          // text 文档分块数
	imagesChunkCount    int          // image 文档分块数
	tagsChunkCount      int          // 标签向量分块数
	lastFileModTime     time.Time    // metadata.json 最近加载时间，用于跨进程一致性检测
}

// PreviewEntry 文件预览条目，包含 MIME 类型和文件类别
type PreviewEntry struct {
	MIME     string // MIME 类型
	Category string // 文件类别: image / video / text
}

// KnowledgeDB 知识库结构体（SQLite）
// 职责：SQL 连接管理、批量操作、表结构管理、信息查询
type KnowledgeDB struct {
	knowledgeDB          *sql.DB // 知识库 SQL 连接
	knowledgeInitialized bool    // 知识库是否初始化完成
}

// MemoryDB 记忆库结构体（多集合架构，扁平化存储，v2 标签向量中介检索）
// 职责：嵌入服务连接、LLM 标签生成、集合管理、记忆 CRUD、维度锁定、持久化
// 存储布局：<baseDir>/<collectionName>/{metadata.json, documents_*.json, images_*.json, tags_*.json}
type MemoryDB struct {
	memoryInitialized bool                   // 记忆库实例是否初始化完成
	embeddingBaseURL  string                 // 嵌入服务 base_url（OpenAI 兼容）
	embeddingAPIKey   string                 // 嵌入服务 API Key
	llmBaseURL        string                 // LLM 标签生成服务 base_url
	llmAPIKey         string                 // LLM 标签生成服务 API Key
	multimodalModel   string                 // 多模态模型名（用于标签生成）
	httpClient        *http.Client           // HTTP 客户端（嵌入 + LLM 共享）
	llmMu             sync.Mutex             // LLM 调用互斥锁（严格单线程）
	baseDir           string                 // 记忆存储根目录绝对路径
	collections       map[string]*Collection // 集合名 → 集合实例
	collectionsMu     sync.RWMutex           // collections map 读写锁
}

// OrganizeOperation 单个整理操作
type OrganizeOperation struct {
	Type   string `json:"type"`             // 操作类型: move, rename, merge, delete
	Source string `json:"source"`           // 源路径（相对于工作目录）
	Target string `json:"target,omitempty"` // 目标路径（相对于工作目录），delete 操作不需要
}

// OrganizeResult 单个操作结果
type OrganizeResult struct {
	Success bool   `json:"success"`
	Type    string `json:"type"`
	Source  string `json:"source"`
	Target  string `json:"target,omitempty"`
	Error   string `json:"error,omitempty"`
}

// OrganizeRequest 批量整理请求
type OrganizeRequest struct {
	BasePath   string              `json:"base_path"`  // 工作目录基础路径（相对于 LocalDir）
	Operations []OrganizeOperation `json:"operations"` // 操作列表
}

// OrganizeResponse 批量整理响应
type OrganizeResponse struct {
	Success      bool             `json:"success"`
	Results      []OrganizeResult `json:"results"`
	Total        int              `json:"total"`
	SuccessCount int              `json:"success_count"`
	FailCount    int              `json:"fail_count"`
	Error        string           `json:"error,omitempty"`
}
