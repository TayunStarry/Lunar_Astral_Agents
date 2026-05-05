package module

import (
	"database/sql"
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

// Database 数据库封装结构
type Database struct {
	db *sql.DB
}

// OperationResult 单个操作结果
type OperationResult struct {
	Success      bool             `json:"success"`
	Error        string           `json:"error,omitempty"`
	Operation    string           `json:"operation"`
	Rows         []map[string]any `json:"rows,omitempty"`
	AffectedRows int64            `json:"affected_rows,omitempty"`
	LastInsertID int64            `json:"last_insert_id,omitempty"`
	Table        string           `json:"table,omitempty"`
	Structure    []map[string]any `json:"structure,omitempty"`
	Tables       []string         `json:"tables,omitempty"`
	Count        int64            `json:"count,omitempty"`
}

// BatchResult 批量操作结果
type BatchResult struct {
	Success    bool              `json:"success"`
	Error      string            `json:"error,omitempty"`
	Results    []OperationResult `json:"results"`
	TotalTime  int64             `json:"total_time_ms"`
	Operations int               `json:"operations"`
}

// ColumnDefinition 列定义
type ColumnDefinition struct {
	Name          string      `json:"name"`
	Type          string      `json:"type"`
	PrimaryKey    bool        `json:"primary_key,omitempty"`
	AutoIncrement bool        `json:"auto_increment,omitempty"`
	NotNull       bool        `json:"not_null,omitempty"`
	Unique        bool        `json:"unique,omitempty"`
	Default       interface{} `json:"default,omitempty"`
}

// TableDefinition 表定义
type TableDefinition struct {
	Columns []ColumnDefinition `json:"columns"`
	Indexes []IndexDefinition  `json:"indexes,omitempty"`
}

// IndexDefinition 索引定义
type IndexDefinition struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique,omitempty"`
}

// DataOperation 数据操作
type DataOperation struct {
	Type   string                 `json:"type"` // insert, update, delete, select
	Table  string                 `json:"table"`
	Data   map[string]interface{} `json:"data,omitempty"`   // insert/update
	Filter map[string]interface{} `json:"filter,omitempty"` // where条件
	Limit  int                    `json:"limit,omitempty"`
	Offset int                    `json:"offset,omitempty"`
	Order  []map[string]string    `json:"order,omitempty"` // [{column: "id", direction: "asc"}]
}

// TableOperation 表操作
type TableOperation struct {
	Type       string           `json:"type"` // create, drop, truncate
	Table      string           `json:"table,omitempty"`
	Definition *TableDefinition `json:"definition,omitempty"`
}

// InfoOperation 信息操作
type InfoOperation struct {
	Type  string `json:"type"` // tables, structure, count
	Table string `json:"table,omitempty"`
}

// DatabaseRequest 数据库请求
type DatabaseRequest struct {
	Operations  []interface{} `json:"operations"` // 可以是DataOperation, TableOperation, InfoOperation
	Transaction bool          `json:"transaction,omitempty"`
}
