package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	LoggerGeneral.SetDevMode(*GeneralConfig.Developer)
}

// InitKnowledgeDB 初始化知识库（SQLite）
// 创建数据库目录、打开连接、设置 PRAGMA，并赋值给全局 KnowledgeDatabase 实例
func InitKnowledgeDB(dbPath string) error {
	if KnowledgeDatabase != nil && KnowledgeDatabase.knowledgeInitialized {
		return nil
	}

	db := &KnowledgeDB{}
	if err := db.initKnowledge(dbPath); err != nil {
		return fmt.Errorf("初始化知识库失败: %v", err)
	}

	KnowledgeDatabase = db
	LoggerGeneral.Info("FileManager", "知识库初始化完成: %s", dbPath)
	return nil
}

// initKnowledge 打开 SQLite 连接并设置连接池与 PRAGMA
func (d *KnowledgeDB) initKnowledge(dbPath string) error {
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
		LoggerGeneral.Error("FileManager", "设置SQLite同步模式失败: %v", err)
	}
	_, err = db.Exec("PRAGMA cache_size = 10000")
	if err != nil {
		LoggerGeneral.Error("FileManager", "设置SQLite缓存大小失败: %v", err)
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		LoggerGeneral.Error("FileManager", "启用外键约束失败: %v", err)
	}

	d.knowledgeDB = db
	d.knowledgeInitialized = true
	return nil
}

// IsKnowledgeInitialized 返回知识库是否已初始化
func (d *KnowledgeDB) IsKnowledgeInitialized() bool {
	return d != nil && d.knowledgeInitialized
}

// Close 关闭知识库连接
func (d *KnowledgeDB) Close() error {
	if d.knowledgeDB != nil {
		if err := d.knowledgeDB.Close(); err != nil {
			return err
		}
	}
	if d.webSearchCacheDB != nil {
		if err := d.webSearchCacheDB.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Ping 测试知识库连接是否存活
func (d *KnowledgeDB) Ping() error {
	if d.knowledgeDB == nil {
		return fmt.Errorf("知识库未初始化")
	}
	_, err := d.knowledgeDB.Exec("SELECT 1")
	return err
}

// =============================================================================
// 原生 SQL 执行
// 数据操作统一回归原生 SQL 语句，不再使用结构化 JSON 操作封装
// =============================================================================

// executeRawSQL 执行原生 SQL
// 支持带参数的占位符（params 为 []any），自动区分查询类与写操作类语句
// dbHandle 为执行目标连接（knowledgeDB 或 webSearchCacheDB）
func (d *KnowledgeDB) executeRawSQL(dbHandle *sql.DB, op map[string]interface{}, tx *sql.Tx) OperationResult {
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

// =============================================================================
// 全局包装函数 — 知识库
// =============================================================================

// EnsureKnowledgeInitialized 确保知识库已初始化并连接可用
func EnsureKnowledgeInitialized() error {
	if KnowledgeDatabase == nil || !KnowledgeDatabase.IsKnowledgeInitialized() {
		if err := InitKnowledgeDB(*GeneralConfig.KnowledgeDBPath); err != nil {
			return err
		}
	}
	if err := KnowledgeDatabase.Ping(); err != nil {
		return fmt.Errorf("知识库连接失败: %v", err)
	}
	return nil
}

// ExecuteSQL 全局包装 — 原生 SQL 直接执行（默认知识库数据源）
// 自动区分查询类（返回 rows）与写操作类（返回 affected_rows / last_insert_id）
func ExecuteSQL(stmt string, params []any) *BatchResult {
	return ExecuteSQLOn("", stmt, params)
}

// ExecuteSQLOn 指定数据源执行原生 SQL
// db 为空或 "knowledge" 时使用知识库（knowledge.db）；
// "web_search_cache" 时使用网络搜索页面摘要缓存（web_search_cache.db，Web-LTP 的派生缓存）；
// 其他取值返回错误。执行语义与 ExecuteSQL 一致。
func ExecuteSQLOn(db string, stmt string, params []any) *BatchResult {
	startTime := time.Now()
	result := &BatchResult{
		Success:    true,
		TotalTime:  0,
		Operations: 1,
		Results:    []OperationResult{{Success: false, Operation: "sql"}},
	}

	var target *sql.DB
	switch db {
	case "", "knowledge":
		if err := EnsureKnowledgeInitialized(); err != nil {
			result.Success = false
			result.Error = err.Error()
			result.Results[0].Error = err.Error()
			return result
		}
		target = KnowledgeDatabase.knowledgeDB
	case "web_search_cache":
		if err := EnsureWebSearchCacheInitialized(); err != nil {
			result.Success = false
			result.Error = err.Error()
			result.Results[0].Error = err.Error()
			return result
		}
		target = KnowledgeDatabase.webSearchCacheDB
	default:
		err := fmt.Errorf("未知的数据源: %q（可选 knowledge / web_search_cache）", db)
		result.Success = false
		result.Error = err.Error()
		result.Results[0].Error = err.Error()
		return result
	}

	op := map[string]any{"type": "sql", "sql": stmt}
	if params != nil {
		op["params"] = params
	}

	opResult := KnowledgeDatabase.executeRawSQL(target, op, nil)
	result.Success = opResult.Success
	result.Error = opResult.Error
	result.Results[0] = opResult
	result.TotalTime = time.Since(startTime).Milliseconds()
	return result
}

// EnsureWebSearchCacheInitialized 确保网络搜索摘要缓存连接可用（懒加载）。
// 缓存文件由 Web-LTP 在首次网络搜索时创建；文件不存在时返回可读错误而不创建空库。
func EnsureWebSearchCacheInitialized() error {
	if KnowledgeDatabase == nil {
		KnowledgeDatabase = &KnowledgeDB{}
	}
	if KnowledgeDatabase.webSearchCacheDB != nil {
		if err := KnowledgeDatabase.webSearchCacheDB.Ping(); err == nil {
			return nil
		}
		// 连接失效（如文件被删除后重建），关闭旧连接重新打开
		KnowledgeDatabase.webSearchCacheDB.Close()
		KnowledgeDatabase.webSearchCacheDB = nil
	}
	return KnowledgeDatabase.initWebSearchCacheDB()
}

// initWebSearchCacheDB 打开网络搜索摘要缓存连接（WAL 模式，小连接池）
func (d *KnowledgeDB) initWebSearchCacheDB() error {
	dbPath := *GeneralConfig.WebSearchCacheDBPath
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("网络搜索缓存数据库不存在（完成一次网络搜索后自动创建）: %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("连接搜索缓存SQLite失败: %v", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("测试搜索缓存SQLite连接失败: %v", err)
	}

	d.webSearchCacheDB = db
	LoggerGeneral.Info("FileManager", "网络搜索摘要缓存连接完成: %s", dbPath)
	return nil
}