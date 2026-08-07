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

// memoryMessage 记忆库查询返回的兼容消息结构（仅用于 MemoryQueryMessages 的 JSON 编码）
type memoryMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 消息内容
}

// MemoryQueryResult 记忆库查询结果（含相似度分数）
type MemoryQueryResult struct {
	ID         string  `json:"id"`         // 文档 ID
	Role       string  `json:"role"`       // 消息角色
	Content    string  `json:"content"`    // 消息内容
	Similarity float32 `json:"similarity"` // 余弦相似度分数 [-1, 1]，越高越相关
}

// DocumentEntry 文档条目 — 用于前端分页列表（不含嵌入向量，避免传输开销）
type DocumentEntry struct {
	ID      string `json:"id"`      // 文档条目 ID
	Role    string `json:"role"`    // 文档条目角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 文档条目内容
}

// MemoryDocument 记忆库文档 — 自行实现的存储单元，含嵌入向量
type MemoryDocument struct {
	ID        string    `json:"id"`        // 文档 ID
	Role      string    `json:"role"`      // 消息角色，user/assistant/system
	Content   string    `json:"content"`   // 原始文本内容
	Embedding []float32 `json:"embedding"` // 嵌入向量（由 /v1/embeddings 生成）
}

// contentEntry 内容分块条目 — 对应 contents_NNNN.json 中的单条记录
// 仅含标识与文本，不含嵌入向量，与 embeddingEntry 通过 ID 关联
type contentEntry struct {
	ID      string `json:"id"`      // 文档 ID（与 embeddingEntry 一一对应）
	Role    string `json:"role"`    // 消息角色，user/assistant/system
	Content string `json:"content"` // 原始文本内容
}

// embeddingEntry 嵌入向量分块条目 — 对应 embeddings_NNNN.json 中的单条记录
// 仅含标识与向量，与 contentEntry 通过 ID 关联
type embeddingEntry struct {
	ID        string    `json:"id"`        // 文档 ID（与 contentEntry 一一对应）
	Embedding []float32 `json:"embedding"` // 嵌入向量
}

// collectionMeta 集合元数据，持久化到 metadata.json
type collectionMeta struct {
	Model      string `json:"model"`                 // 锁定的嵌入模型名
	Dimension  int    `json:"dimension"`             // 锁定的向量维度（探针文本确定）
	ChunkCount int    `json:"chunk_count,omitempty"` // 分块对数（0 表示空集合或旧格式）
	Type       string `json:"type,omitempty"`        // 集合类型："text" 或 "image"（空值等价于 "text"）
}

// Collection 单个记忆集合 — 集合级锁定的模型与维度
// 文档 ID 采用 UUID v4 格式（由 generateUUID 生成），旧版 msg-N 格式 ID 在加载时保留原值
// text 类型存储布局：<collDir>/contents_NNNN.json + embeddings_NNNN.json（分块存储，每块 ≤100 条）
// image 类型存储布局：<collDir>/base64_NNNN.json + embeddings_NNNN.json（分块存储，每块 ≤100 条）
type Collection struct {
	Name            string           // 集合名
	Model           string           // 锁定的嵌入模型名
	Dimension       int              // 锁定的向量维度（探针文本确定，0 表示未确定）
	CollectionType  string           // 集合类型："text" 或 "image"
	Documents       []MemoryDocument // 文本文档列表（含嵌入向量），text 类型使用
	ImageDocuments  []ImageDocument  // 图片文档列表（含三元嵌入向量），image 类型使用
	mu              sync.RWMutex     // 文档读写锁
	collDir         string           // 集合目录绝对路径
	metaPath        string           // metadata.json 路径
	chunkCount      int              // 当前分块对数（0 表示空集合）
	lastFileModTime time.Time        // metadata.json 最近一次已加载的修改时间，用于跨进程一致性检测
}

// ImageDocument 图片记忆库文档 — 包含 base64 图片数据与三元嵌入向量
// 三个嵌入向量分别对应：情绪描述、色彩风格描述、主要内容描述
type ImageDocument struct {
	ID         string       `json:"id"`         // 文档 ID（UUID v4 格式）
	Image      string       `json:"image"`      // 图片 base64 编码数据
	Embeddings [3][]float32 `json:"embeddings"` // 三个嵌入向量：[情绪, 色彩风格, 主要内容]
}

// base64Entry base64 分块条目 — 对应 base64_NNNN.json 中的单条记录
// 仅含标识与图片 base64 数据，与 imageEmbeddingEntry 通过 ID 关联
type base64Entry struct {
	ID    string `json:"id"`    // 文档 ID（与 imageEmbeddingEntry 一一对应）
	Image string `json:"image"` // 图片 base64 编码数据
}

// imageEmbeddingEntry 图片嵌入向量分块条目 — 对应 embeddings_NNNN.json 中的单条记录（image 类型）
// 与 text 类型的 embeddingEntry 不同，此类型存储三个嵌入向量组成的数组
type imageEmbeddingEntry struct {
	ID         string       `json:"id"`         // 文档 ID（与 base64Entry 一一对应）
	Embeddings [3][]float32 `json:"embeddings"` // 三个嵌入向量：[情绪, 色彩风格, 主要内容]
}

// ImageQueryResult 图片记忆库查询结果（含相似度分数与最终评分）
type ImageQueryResult struct {
	ID         string  `json:"id"`          // 文档 ID
	Image      string  `json:"image"`       // 图片 base64 编码数据
	BaseScore  float32 `json:"base_score"`  // 基础评分（三个向量相似度平均值）
	FinalScore float32 `json:"final_score"` // 最终评分（tok5 加权后）
	BoostLevel int     `json:"boost_level"` // 加权等级：0/1/2/3（对应 ×1.0/×1.3/×1.6/×2.0）
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

// MemoryDB 记忆库结构体（多集合架构，扁平化存储）
// 职责：嵌入服务连接、集合管理、记忆 CRUD、维度锁定、持久化
// 存储布局：<baseDir>/<collectionName>/{documents.json, metadata.json}
type MemoryDB struct {
	memoryInitialized bool                   // 记忆库实例是否初始化完成
	embeddingBaseURL  string                 // 嵌入服务 base_url（OpenAI 兼容）
	embeddingAPIKey   string                 // 嵌入服务 API Key
	httpClient        *http.Client           // 嵌入服务 HTTP 客户端（所有集合共享）
	baseDir           string                 // 记忆存储根目录绝对路径（集合目录的直接父级）
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
