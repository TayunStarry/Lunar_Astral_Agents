package module

import (
	"config"
	"database/sql"
	"logger"
	"sync"
	"time"

	chromem "github.com/philippgille/chromem-go"
)

func init() {
	logger.SetDevMode(*config.Developer)
}

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

// DatabaseRequest 数据库请求
type DatabaseRequest struct {
	Operations  []interface{} `json:"operations"`            // 数据库操作列表 ，每个元素可以是DataOperation, TableOperation, InfoOperation
	Transaction bool          `json:"transaction,omitempty"` // 是否开启事务，默认 false
}

// chromemMessage 表示 chromem-go 中的消息结构
type chromemMessage struct {
	Role    string `json:"role"`    // 消息角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 消息内容
}

// VectorQueryResult 向量查询结果（含相似度分数）
type VectorQueryResult struct {
	ID         string  `json:"id"`         // 文档 ID
	Role       string  `json:"role"`       // 消息角色
	Content    string  `json:"content"`    // 消息内容
	Similarity float32 `json:"similarity"` // 余弦相似度分数 [-1, 1]，越高越相关
}

// DocumentEntry 文档条目 — 用于前端分页列表
type DocumentEntry struct {
	ID      string `json:"id"`      // 文档条目 ID
	Role    string `json:"role"`    // 文档条目角色，例如 "user" 或 "assistant"
	Content string `json:"content"` // 文档条目内容
}

// UnifiedDB 统一数据库结构体
type UnifiedDB struct {
	sqlDB             *sql.DB             // SQL 数据库连接
	chromemDB         *chromem.DB         // 向量数据库连接
	collection        *chromem.Collection // 向量数据库集合
	sqlInitialized    bool                // SQL 数据库是否初始化完成
	vectorInitialized bool                // 向量数据库是否初始化完成
	documentEntries   []DocumentEntry     // 文档条目列表，用于前端分页列表
	documentEntriesMu sync.RWMutex        // 文档条目列表的读写锁，用于并发访问
	entriesFilePath   string              // 文档条目列表的 JSON 文件路径，用于存储和加载
	messageIDCounter  int                 // 消息 ID计数器，用于生成唯一的消息 ID
}
