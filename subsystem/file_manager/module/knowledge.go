package module

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	d.fileDBs = map[string]*sql.DB{}
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
	d.fileDBsMu.Lock()
	for name, db := range d.fileDBs {
		if err := db.Close(); err != nil {
			d.fileDBsMu.Unlock()
			return fmt.Errorf("关闭数据库 %s 失败: %v", name, err)
		}
	}
	d.fileDBs = map[string]*sql.DB{}
	d.fileDBsMu.Unlock()
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
// "web_search_cache" 时使用网络搜索页面摘要缓存（web_search_cache.db，Web-LTP 的派生缓存，遗留只读）；
// 其他取值视为 database 目录内的 *.db 文件路径（去 .db，支持子目录如 "sub/foo"）：文件必须已存在
// 且为有效 SQLite 库，连接懒打开并缓存复用，不创建新文件。执行语义与 ExecuteSQL 一致。
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
		fileDB, ferr := KnowledgeDatabase.ensureFileDB(db)
		if ferr != nil {
			result.Success = false
			result.Error = ferr.Error()
			result.Results[0].Error = ferr.Error()
			return result
		}
		target = fileDB
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

// =============================================================================
// 知识条目通道（月华 knowledge 存储复用）与任意 *.db 数据源
// =============================================================================

// knowledgeValidName 库名/表名白名单（防路径穿越与表名拼接注入）
var knowledgeValidName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// KnowledgeLoadEntries 读取指定表的全部条目（月华 knowledge 存储通道）。
// 条目格式 [key, text][]，按 rowid 稳定排序；表不存在时返回空数组。
func KnowledgeLoadEntries(table string) ([][]string, error) {
	if !knowledgeValidName.MatchString(table) {
		return nil, fmt.Errorf("无效的知识库表名: %q", table)
	}
	if err := EnsureKnowledgeInitialized(); err != nil {
		return nil, err
	}
	rows, err := KnowledgeDatabase.knowledgeDB.Query("SELECT key, value FROM " + table + " ORDER BY rowid")
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return [][]string{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	entries := [][]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		entries = append(entries, []string{k, v})
	}
	return entries, rows.Err()
}

// KnowledgeReplaceEntries 以「全表替换」语义写入指定表（月华 knowledge 存储通道）。
// 事务内：建表（幂等）→ 清空 → 逐条插入；任一步失败整体回滚。
func KnowledgeReplaceEntries(table string, entries [][]string) error {
	if !knowledgeValidName.MatchString(table) {
		return fmt.Errorf("无效的知识库表名: %q", table)
	}
	if err := EnsureKnowledgeInitialized(); err != nil {
		return err
	}
	tx, err := KnowledgeDatabase.knowledgeDB.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("CREATE TABLE IF NOT EXISTS " + table + " (key TEXT PRIMARY KEY, value TEXT)"); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec("DELETE FROM " + table); err != nil {
		_ = tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare("INSERT OR REPLACE INTO " + table + " (key, value) VALUES (?, ?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, pair := range entries {
		if len(pair) == 0 {
			continue
		}
		key := pair[0]
		val := ""
		if len(pair) >= 2 {
			val = pair[1]
		}
		if _, err := stmt.Exec(key, val); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ensureFileDB 打开（或复用）database 目录内指定 *.db 的连接（懒加载，不创建文件）。
// name 为相对 database 目录的文件路径去 .db（顶层如 "web_search_cache"，子目录如 "sub/foo"）；
// 逐段校验名称白名单、文件存在性与 SQLite 魔数，全部通过才打开。
func (d *KnowledgeDB) ensureFileDB(name string) (*sql.DB, error) {
	if d == nil {
		KnowledgeDatabase = &KnowledgeDB{}
		d = KnowledgeDatabase
	}
	rel, err := knowledgeRelPath(name)
	if err != nil {
		return nil, err
	}
	d.fileDBsMu.Lock()
	defer d.fileDBsMu.Unlock()
	if db, ok := d.fileDBs[name]; ok {
		return db, nil
	}
	dbDir := filepath.Clean(filepath.Dir(*GeneralConfig.KnowledgeDBPath))
	dbPath := filepath.Clean(filepath.Join(dbDir, rel+".db"))
	// 防穿越复核：最终路径必须仍位于 database 目录内
	if !strings.HasPrefix(dbPath, dbDir+string(os.PathSeparator)) {
		return nil, fmt.Errorf("非法数据库路径: %s", name)
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("数据库文件不存在: %s", dbPath)
	}
	// 验证 SQLite 魔数，拒绝误传非 SQLite 文件
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("读取数据库失败: %v", err)
	}
	magic := make([]byte, 16)
	_, rerr := io.ReadFull(f, magic)
	f.Close()
	if rerr != nil {
		return nil, fmt.Errorf("读取数据库头失败: %s", dbPath)
	}
	if string(magic) != "SQLite format 3\x00" {
		return nil, fmt.Errorf("非有效的 SQLite 数据库: %s", dbPath)
	}
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("连接 SQLite 失败: %v", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试 SQLite 连接失败: %v", err)
	}
	if d.fileDBs == nil {
		d.fileDBs = map[string]*sql.DB{}
	}
	d.fileDBs[name] = db
	LoggerGeneral.Info("FileManager", "数据源连接完成: %s", dbPath)
	return db, nil
}

// knowledgeRelPath 校验并规整相对 database 目录的库文件路径（不含 .db 后缀）。
// 仅允许「段/段/…」形式，每段为字母数字-_，返回以 / 分隔的规整路径。
func knowledgeRelPath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("数据库名不能为空")
	}
	segs := strings.Split(filepath.ToSlash(name), "/")
	for _, seg := range segs {
		if !knowledgeValidName.MatchString(seg) {
			return "", fmt.Errorf("非法数据库名: %q（仅允许字母数字-_）", name)
		}
	}
	return strings.Join(segs, "/"), nil
}