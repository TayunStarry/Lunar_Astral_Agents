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
// v3 记忆库类型定义 — 文档引用标签 UUID 架构
// 核心变更：引用方向反转，TagVector 不再存储文档 UUID
// =============================================================================

// memoryMessage 记忆库查询返回的兼容消息结构（仅用于 MemoryQueryMessages 的 JSON 编码）
type memoryMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 消息内容
}

// MemoryQueryResult 记忆库查询结果（含相似度分数）
// v3: Similarity 字段表示匹配标签的余弦相似度平均值
type MemoryQueryResult struct {
	ID         string  `json:"id"`              // 文档 ID
	Role       string  `json:"role"`            // 消息角色，image 文档为 "image"
	Content    string  `json:"content"`         // 消息内容，image 文档为空
	Image      string  `json:"image,omitempty"` // 图片 base64 数据，仅 image 文档
	Similarity float32 `json:"similarity"`      // 匹配标签余弦相似度平均值
}

// DocumentEntry 文档条目 — 用于前端分页列表（不含嵌入向量，避免传输开销）
type DocumentEntry struct {
	ID      string `json:"id"`              // 文档条目 ID
	Role    string `json:"role"`            // 文档条目角色，"image" 表示图片文档
	Content string `json:"content"`         // 文档条目内容，image 文档为空
	Image   string `json:"image,omitempty"` // 图片 base64 数据，仅 image 文档
}

// Document 统一文档结构（text 和 image 共用）
// v3: 新增 TagUUIDs 字段，存储引用的标签向量 UUID
type Document struct {
	ID       string   `json:"id"`                  // 文档 UUID v4
	Role     string   `json:"role,omitempty"`      // 消息角色，text 文档使用
	Content  string   `json:"content,omitempty"`   // 文本内容，text 文档使用
	Image    string   `json:"image,omitempty"`     // 图片 base64 数据，image 文档使用
	TagUUIDs []string `json:"tag_uuids,omitempty"` // v3: 引用的标签向量 UUID 列表
}

// TagVector 标签向量条目 — 标签文本的嵌入向量，拥有独立 UUID
// v3: 不再存储关联文档 UUID，引用关系由 Document.TagUUIDs 维护
type TagVector struct {
	UUID      string    `json:"uuid"`      // v3: 标签向量唯一标识
	Tag       string    `json:"tag"`       // 标签文本（保留，便于调试和可解释性）
	Embedding []float32 `json:"embedding"` // 标签文本的嵌入向量
}

// collectionMeta v3 集合元数据，持久化到 metadata.json
type collectionMeta struct {
	EmbeddingModel      string `json:"embedding_model"`       // 锁定的嵌入模型名
	EmbeddingDimension  int    `json:"embedding_dimension"`   // 锁定的向量维度
	MultimodalModel     string `json:"multimodal_model"`      // 标签生成多模态模型名
	Type                string `json:"type"`                  // 集合类型："text" 或 "image"
	Version             int    `json:"version"`               // 数据格式版本号（v3 = 3）
	DocumentsChunkCount int    `json:"documents_chunk_count"` // text 文档分块数
	ImagesChunkCount    int    `json:"images_chunk_count"`    // image 文档分块数
	TagsChunkCount      int    `json:"tags_chunk_count"`      // 标签向量分块数
}

// Collection 单个记忆集合 — v3 文档引用标签 UUID 架构
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

// MemoryDB 记忆库结构体（多集合架构，扁平化存储，v3 文档引用标签 UUID 架构）
// 职责：嵌入服务连接、LLM 标签生成、集合管理、记忆 CRUD、维度锁定、持久化
// 模型配置（URL、模型名、API Key）从 config 模块（lunar_config.json）读取
// 存储布局：<baseDir>/<collectionName>/{metadata.json, documents_*.json, images_*.json, tags_*.json}
type MemoryDB struct {
	memoryInitialized bool                   // 记忆库实例是否初始化完成
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

// =============================================================================
// 文件移动类型定义（/file/move）
// =============================================================================

// MoveItemRequest 文件移动请求
// Sources 为相对 LocalDir 的源路径列表；TargetDir 为目标文件夹（相对 LocalDir，空表示根目录）
// ConflictStrategy 冲突策略: "ask"（预检冲突并返回，不执行移动）/ "auto_rename"（自动添加序号重命名）/ "overwrite"（覆盖同名项）
type MoveItemRequest struct {
	Sources          []string `json:"sources"`           // 源路径列表（相对 LocalDir）
	TargetDir        string   `json:"target_dir"`        // 目标文件夹（相对 LocalDir，空表示根目录）
	ConflictStrategy string   `json:"conflict_strategy"` // 冲突策略: ask / auto_rename / overwrite
	CreateDirs       bool     `json:"create_dirs"`       // 目标文件夹不存在时是否自动创建
}

// MoveConflict 移动冲突项（ask 预检时返回）
type MoveConflict struct {
	Source string `json:"source"` // 源路径
	Target string `json:"target"` // 目标路径
	IsDir  bool   `json:"is_dir"` // 冲突项是否为文件夹
}

// MoveItemResult 单个移动结果
type MoveItemResult struct {
	Source   string `json:"source"`   // 源路径
	Target   string `json:"target"`   // 最终目标路径（冲突重命名后为实际路径）
	Conflict bool   `json:"conflict"` // 是否检测到同名冲突
	Renamed  bool   `json:"renamed"`  // 是否因冲突自动重命名
	Error    string `json:"error,omitempty"`
}

// MoveResponse 文件移动响应
type MoveResponse struct {
	Success   bool             `json:"success"`
	Results   []MoveItemResult `json:"results,omitempty"`
	Conflicts []MoveConflict   `json:"conflicts,omitempty"` // ask 预检冲突列表
	Error     string           `json:"error,omitempty"`
}

// ZipEntryInfo ZIP 压缩包内条目信息
type ZipEntryInfo struct {
	Name       string `json:"name"`       // 条目路径（含子目录）
	Size       int64  `json:"size"`       // 解压后大小
	Compressed int64  `json:"compressed"` // 压缩后大小
	IsDir      bool   `json:"isDir"`      // 是否为目录
}

// ZipMetadataRequest ZIP 元数据查询请求
type ZipMetadataRequest struct {
	Path string `json:"path"` // ZIP 文件路径（相对 LocalDir）
}

// ZipMetadataResponse ZIP 元数据查询响应
type ZipMetadataResponse struct {
	Success   bool           `json:"success"`
	Error     string         `json:"error,omitempty"`
	Path      string         `json:"path"`       // 压缩包相对 LocalDir 的路径
	FileCount int            `json:"file_count"` // 条目总数
	TotalSize int64          `json:"total_size"` // 解压后总大小
	ZipSize   int64          `json:"zip_size"`   // 压缩包文件大小
	Entries   []ZipEntryInfo `json:"entries"`    // 条目列表
}

// ExtractZipRequest ZIP 解压请求
type ExtractZipRequest struct {
	Path      string `json:"path"`       // ZIP 文件路径（相对 LocalDir）
	TargetDir string `json:"target_dir"` // 目标目录（相对 LocalDir，空表示 LocalDir 根目录）
}

// ExtractZipResponse ZIP 解压响应
type ExtractZipResponse struct {
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	TargetDir string `json:"target_dir"` // 实际解压目录（相对 LocalDir）
	FileCount int    `json:"file_count"` // 解压出的文件数
}

// CreateZipRequest 服务端压缩请求
type CreateZipRequest struct {
	Paths    []string `json:"paths"`     // 相对 LocalDir 的文件/目录路径列表
	ZipName  string   `json:"zip_name"`  // ZIP 文件名（缺省为 archive.zip，自动补全 .zip 后缀）
	SavePath string   `json:"save_path"` // 保存目录（相对 LocalDir，空表示 LocalDir 根目录）
}

// ZipCreateResponse 服务端压缩响应
type ZipCreateResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Path    string `json:"path"` // 压缩包相对 LocalDir 的路径
	Name    string `json:"name"` // 压缩包文件名
	Size    int    `json:"size"` // 压缩包大小（字节）
}

// HashRenameRequest 哈希命名请求
type HashRenameRequest struct {
	Path string `json:"path"` // 目标目录路径（相对 LocalDir，空表示根目录）
}

// HashRenameItem 单个文件的哈希命名结果
type HashRenameItem struct {
	OldName   string `json:"old_name"`  // 原文件名
	NewName   string `json:"new_name"`  // 新文件名（MD5 前 16 位 + 原扩展名）
	Hash      string `json:"hash"`      // 文件内容 MD5 的前 16 位
	Duplicate bool   `json:"duplicate"` // 是否因重名追加了 '+'
	Unchanged bool   `json:"unchanged"` // 是否无需改动（已是哈希名）
}

// HashRenameResponse 哈希命名响应
type HashRenameResponse struct {
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
	Results []HashRenameItem `json:"results"` // 全部文件的处理结果
	Renamed int              `json:"renamed"` // 实际重命名的文件数
}
