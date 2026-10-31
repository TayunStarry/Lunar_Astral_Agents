package knowledge

// type.go 全部类型定义（结构体、接口、别名）集中于此。

import (
	"database/sql"
	"sync"
)

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

// Store 通用知识库实例（knowledge.db）
// 职责：SQL 连接管理、任意 *.db 数据源懒打开；内建缓存与索引按领域落到
// media / web / text / character 库（见 domain_db.go）。
type Store struct {
	db          *sql.DB            // 知识库 SQL 连接
	initialized bool               // 知识库是否初始化完成
	fileDBs     map[string]*sql.DB // 任意 *.db 懒打开连接缓存（键为数据库文件名去 .db）
	fileDBsMu   sync.Mutex         // fileDBs 并发打开互斥锁
}

// domainDB 一个领域库的懒加载连接。
type domainDB struct {
	label      string        // 日志用中文名
	path       func() string // 运行期读取配置（测试可改 flag）
	schema     []string      // 幂等建表语句
	mu         sync.Mutex    // 串行化打开 / 重开 / 关闭
	handle     *sql.DB       // 已打开的连接
	openedPath string        // handle 对应的规整路径（检测配置变化）
}
