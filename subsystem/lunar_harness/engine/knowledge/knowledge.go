package knowledge

// knowledge.go 知识库系统入口 —— 通用知识库 knowledge.db 与原生 SQL 执行通道。
//
// 本包统一收口全部 SQLite 数据操作：通用知识库、各领域库（media / web / text / character，
// 以及由其它模块登记的 archive）、database 目录内的任意 *.db 数据源。
// 上层智能体与 HTTP 端点只引用本包导出的接口，不再自行拼表结构与 SQL 路由。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	logger.SetDevMode(config.Developer)
	// 内建领域库随包登记；archive 由 persistence 在资源目录就绪时登记
	RegisterDomain("media", mediaDB.open)
	RegisterDomain("web", webDB.open)
	RegisterDomain("text", textDB.open)
	RegisterDomain("character", characterDB.open)
}

// InitKnowledge 初始化通用知识库（SQLite）
// 创建数据库目录、打开连接、设置 PRAGMA，并把历史内建表迁往各自领域库。
func InitKnowledge(dbPath string) error {
	if store.IsInitialized() {
		return nil
	}

	instance := &Store{}
	if err := instance.init(dbPath); err != nil {
		return fmt.Errorf("初始化知识库失败: %v", err)
	}

	store = instance
	logger.Info("FileManager", "知识库初始化完成: %s", dbPath)
	// 历史版本把媒体摘要、网页缓存、角色库、文件索引都塞在 knowledge.db：
	// 启动时一次性搬到各自领域库，旧表搬完即删（先整库备份，失败则保留旧表待下次重试）
	store.migrateLegacyTables(dbPath)
	return nil
}

// init 打开 SQLite 连接并设置连接池与 PRAGMA
func (d *Store) init(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建数据库目录失败: %v", err)
		}
	}

	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("连接SQLite失败: %v", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err = db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("测试SQLite连接失败: %v", err)
	}

	_, err = db.Exec("PRAGMA synchronous = NORMAL")
	if err != nil {
		logger.Error("FileManager", "设置SQLite同步模式失败: %v", err)
	}
	_, err = db.Exec("PRAGMA cache_size = 10000")
	if err != nil {
		logger.Error("FileManager", "设置SQLite缓存大小失败: %v", err)
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		logger.Error("FileManager", "启用外键约束失败: %v", err)
	}

	d.db = db
	d.initialized = true
	d.fileDBs = map[string]*sql.DB{}
	return nil
}

// IsInitialized 返回知识库是否已初始化
func (d *Store) IsInitialized() bool {
	return d != nil && d.initialized
}

// Close 关闭通用知识库及其任意 *.db 缓存连接（幂等；下次使用会按当前配置重开）
func (d *Store) Close() error {
	if d == nil {
		return nil
	}
	var firstErr error
	if d.db != nil {
		firstErr = d.db.Close()
		d.db = nil
	}
	d.fileDBsMu.Lock()
	for name, db := range d.fileDBs {
		if err := db.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("关闭数据库 %s 失败: %v", name, err)
		}
	}
	d.fileDBs = map[string]*sql.DB{}
	d.fileDBsMu.Unlock()
	d.initialized = false
	return firstErr
}

// Ping 测试知识库连接是否存活
func (d *Store) Ping() error {
	if d == nil || d.db == nil {
		return fmt.Errorf("知识库未初始化")
	}
	_, err := d.db.Exec("SELECT 1")
	return err
}

// ensure 确保知识库已初始化并连接可用
func ensure() error {
	if !store.IsInitialized() {
		if err := InitKnowledge(config.KnowledgeDBPath); err != nil {
			return err
		}
	}
	if err := store.Ping(); err != nil {
		return fmt.Errorf("知识库连接失败: %v", err)
	}
	return nil
}

// =============================================================================
// 原生 SQL 执行
// 数据操作统一回归原生 SQL 语句，不再使用结构化 JSON 操作封装
// =============================================================================

// executeRawSQLOn 在指定连接上执行原生 SQL（不依赖 Store 实例，领域库同样适用）
// 支持带参数的占位符（params 为 []any），自动区分查询类与写操作类语句
func executeRawSQLOn(dbHandle *sql.DB, op map[string]interface{}, tx *sql.Tx) OperationResult {
	stmt, _ := op["sql"].(string)
	stmt = strings.TrimSpace(stmt)

	if stmt == "" {
		return OperationResult{
			Success:   false,
			Error:     "SQL 语句不能为空",
			Operation: "sql",
		}
	}

	var params []interface{}
	if rawParams, ok := op["params"].([]interface{}); ok {
		params = rawParams
	}

	// 根据语句首关键字判断是否需要返回行
	upper := strings.ToUpper(stmt)
	upper = strings.TrimPrefix(upper, "/*")
	isRead := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")

	if isRead {
		var rows *sql.Rows
		var err error
		if tx != nil {
			rows, err = tx.Query(stmt, params...)
		} else {
			rows, err = dbHandle.Query(stmt, params...)
		}
		if err != nil {
			return OperationResult{
				Success:   false,
				Error:     fmt.Sprintf("查询失败: %v", err),
				Operation: "sql",
			}
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return OperationResult{
				Success:   false,
				Error:     fmt.Sprintf("获取列信息失败: %v", err),
				Operation: "sql",
			}
		}

		var resultRows []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range columns {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return OperationResult{
					Success:   false,
					Error:     fmt.Sprintf("扫描行数据失败: %v", err),
					Operation: "sql",
				}
			}
			rowData := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					rowData[col] = string(b)
				} else {
					rowData[col] = val
				}
			}
			resultRows = append(resultRows, rowData)
		}

		return OperationResult{
			Success:      true,
			Operation:    "sql",
			Rows:         resultRows,
			AffectedRows: int64(len(resultRows)),
		}
	}

	// 写操作（INSERT / UPDATE / DELETE / CREATE / DROP 等）
	var result sql.Result
	var err error
	if tx != nil {
		result, err = tx.Exec(stmt, params...)
	} else {
		result, err = dbHandle.Exec(stmt, params...)
	}
	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("执行失败: %v", err),
			Operation: "sql",
		}
	}

	affectedRows, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	return OperationResult{
		Success:      true,
		Operation:    "sql",
		AffectedRows: affectedRows,
		LastInsertID: lastInsertID,
	}
}

// Execute 原生 SQL 直接执行（默认知识库数据源）
// 自动区分查询类（返回 rows）与写操作类（返回 affected_rows / last_insert_id）
func Execute(stmt string, params []any) *BatchResult {
	return ExecuteOn("", stmt, params)
}

// ExecuteOn 指定数据源执行原生 SQL
// db 为空或 "knowledge" 时使用通用知识库（knowledge.db）；
// 已登记的域名（media / web / text / character，以及由其它模块登记的 archive）走对应域库；
// 其他取值视为 database 目录内的 *.db 文件路径（去 .db，支持子目录如 "sub/foo"）：文件必须已存在
// 且为有效 SQLite 库，连接懒打开并缓存复用，不创建新文件。执行语义与 Execute 一致。
func ExecuteOn(db string, stmt string, params []any) *BatchResult {
	startTime := time.Now()
	result := &BatchResult{
		Success:    true,
		TotalTime:  0,
		Operations: 1,
		Results:    []OperationResult{{Success: false, Operation: "sql"}},
	}

	target, openErr := Open(db)
	if openErr != nil {
		result.Success = false
		result.Error = openErr.Error()
		result.Results[0].Error = openErr.Error()
		return result
	}

	op := map[string]any{"type": "sql", "sql": stmt}
	if params != nil {
		op["params"] = params
	}

	opResult := executeRawSQLOn(target, op, nil)
	result.Success = opResult.Success
	result.Error = opResult.Error
	result.Results[0] = opResult
	result.TotalTime = time.Since(startTime).Milliseconds()
	return result
}
