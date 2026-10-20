package persistence

import (
	"database/sql"
	"net/http"
	"sync"
	"time"
)

// FileInfo 文件信息结构体，用于存储文件和目录的相关信息
type FileInfo struct {
	Name         string    `json:"name"`                   // 文件名或目录名
	Size         int64     `json:"size"`                   // 文件大小
	IsDir        bool      `json:"isDir"`                  // 是否为目录
	LastModified time.Time `json:"lastModified"`           // 最后修改时间
	Path         string    `json:"path"`                   // 相对于配置目录的相对路径
	Handle       string    `json:"handle,omitempty"`       // 句柄（内容仓库内文件；目录或无句柄时为空）
	Kind         string    `json:"kind,omitempty"`         // 资源类型码 D/I/V/A/T
	ContentHash  string    `json:"content_hash,omitempty"` // 媒体内容哈希（文本类为空）
}

// SaveOptions 文件写入选项
type SaveOptions struct {
	// Overwrite 目标已存在时是否覆盖；false 时按时间戳生成新版本
	Overwrite bool
	// MustStore 强制写入指定目录（仅对「新内容」生效）。
	// 内容哈希已存在时仍以唯一性为准：不会因 MustStore 制造第二份实体。
	MustStore bool
}

// SaveResult 文件写入结果（含句柄映射信息）
type SaveResult struct {
	Name    string `json:"name"`     // 最终文件名（覆盖/重命名后）
	Path    string `json:"path"`     // 绝对路径
	RelPath string `json:"rel_path"` // 相对 LocalDir 的路径
	Handle  string `json:"handle"`   // 句柄（内容仓库内文件；否则为空）
	Deduped bool   `json:"deduped"`  // 是否复用了既有内容实体
	Size    int64  `json:"size"`     // 文件字节数
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

// =============================================================================
// v5 记忆库类型定义 — 文本/图片混合存储，仅内容嵌入向量
// 核心变更：
//   - 移除标签向量（TagVector）与 LLM 文本标签生成机制，仅图片入库时用多模态生成标签用于嵌入
//   - 文本与图片记忆混合存储在同一个集合，不再区分"文本集合"/"图片集合"
//   - Document 统一以 content（文本）与 base64（图片）字段区分记忆类型，
//     客户端根据两个字段的存在情况自行判定取用哪种内容
// =============================================================================

// MemoryQueryResult 记忆库查询结果（含相似度分数）
// Type 为记忆类型（text/image/audio）；Collection 为内容自动归入的分组集合名。
// 客户端依据 type 取用对应内容，不再按集合名查询。
type MemoryQueryResult struct {
	ID         string  `json:"id"`                   // 文档 ID
	Type       string  `json:"type"`                 // 记忆类型：text / image / audio
	Role       string  `json:"role"`                 // 消息角色，图片记忆为 "image"
	Content    string  `json:"content"`              // 文本内容，文本记忆使用；图片记忆为空
	URL        string  `json:"url,omitempty"`        // 媒体读取地址（multimedia|audios/<集合名> 下的文件）
	Base64     string  `json:"base64,omitempty"`     // 旧版图片内联数据（仅历史数据兼容）
	Collection string  `json:"collection,omitempty"` // 内容所属分组集合名
	Source     string  `json:"source,omitempty"`     // 写入来源标记（如 action；普通对话为空）
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
	Source    string `json:"source,omitempty"`    // 写入来源标记（如 action），用于内部区分动作记忆
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
	EmbeddingModel       string    `json:"embedding_model"`                // 锁定的嵌入模型名
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
	Model                string       // 锁定的嵌入模型名
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

// PreviewEntry 文件预览条目，包含 MIME 类型和文件类别
type PreviewEntry struct {
	MIME     string // MIME 类型
	Category string // 文件类别: image / video / text
}

// KnowledgeDB 知识库结构体（SQLite）
// 职责：SQL 连接管理、批量操作、表结构管理、信息查询
// 通用知识库（knowledge.db）只存用户/插件自建表；内建缓存与索引按领域落到
// media / web / text / character 库（见 domain_db.go）。
// 另支持懒打开 database 目录内任意 *.db（含子目录），作为 /knowledge/ 端点的可选数据源
type KnowledgeDB struct {
	knowledgeDB          *sql.DB            // 知识库 SQL 连接
	knowledgeInitialized bool               // 知识库是否初始化完成
	fileDBs              map[string]*sql.DB // 任意 *.db 懒打开连接缓存（键为数据库文件名去 .db）
	fileDBsMu            sync.Mutex         // fileDBs 并发打开互斥锁
}

// ArchiveDB 统一资源目录结构体（SQLite archive.db，见 docs/adr/0001）
// 职责：句柄分配与解析、资源元数据、段落结构表、跨资源搜索。
// 不存正文（在内容仓库）、不存向量（在记忆库）、不存媒体派生缓存（在 media.db）。
type ArchiveDB struct {
	db          *sql.DB // 资源目录 SQL 连接
	initialized bool    // 是否初始化完成
}

// MemoryDB 记忆库结构体（多集合分组架构）
// 职责：嵌入服务连接、LLM 标签生成、集合管理、记忆 CRUD、维度锁定、持久化
// 模型配置与智能体共用 agent.embedding_* / agent.multimodal_*（不再有独立记忆库模型配置）
// 存储布局：<baseDir>/<collectionName>/{metadata.json, documents_*.json, embeddings_*.json}
type MemoryDB struct {
	initMu            sync.Mutex             // 实例初始化互斥锁（启动/懒加载并发保护）
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
