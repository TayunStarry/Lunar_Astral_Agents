package module

import (
	"config"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"logger"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	chromem "github.com/philippgille/chromem-go"
)

func init() {
	logger.SetDevMode(*config.Developer)
}

func InitUnifiedDB(sqlPath string, vectorDir string) error {
	if Unified != nil && Unified.sqlInitialized && Unified.vectorInitialized {
		return nil
	}

	u := &UnifiedDB{}

	if err := u.initSQL(sqlPath); err != nil {
		return fmt.Errorf("初始化SQL数据库失败: %v", err)
	}

	if err := os.MkdirAll(vectorDir, 0755); err != nil {
		return fmt.Errorf("创建向量数据库目录失败: %v", err)
	}
	u.entriesFilePath = filepath.Join(vectorDir, "entries.json")

	Unified = u
	logger.Info("Storage", "统一数据库 SQL 初始化完成: %s", sqlPath)
	logger.Info("Storage", "统一数据库向量存储目录已就绪: %s", vectorDir)

	return nil
}

func (u *UnifiedDB) initSQL(dbPath string) error {
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
		logger.Error("Storage", "设置SQLite同步模式失败: %v", err)
	}
	_, err = db.Exec("PRAGMA cache_size = 10000")
	if err != nil {
		logger.Error("Storage", "设置SQLite缓存大小失败: %v", err)
	}
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		logger.Error("Storage", "启用外键约束失败: %v", err)
	}

	u.sqlDB = db
	u.sqlInitialized = true
	return nil
}

func (u *UnifiedDB) VectorInit(baseURL string, apiKey string, modelName string) error {
	if u.vectorInitialized {
		return nil
	}

	if u.entriesFilePath == "" {
		return fmt.Errorf("向量数据库未配置存储路径, 请先调用 InitUnifiedDB")
	}

	db, err := chromem.NewPersistentDB(filepath.Dir(u.entriesFilePath), true)
	if err != nil {
		return fmt.Errorf("chromem 创建持久化数据库失败: %v", err)
	}

	embeddingFunc := chromem.NewEmbeddingFuncOpenAICompat(baseURL, apiKey, modelName, nil)

	collection, err := db.GetOrCreateCollection("lunar_messages", nil, embeddingFunc)
	if err != nil {
		return fmt.Errorf("chromem 创建集合失败: %v", err)
	}

	u.chromemDB = db
	u.collection = collection
	u.vectorInitialized = true

	u.loadEntriesFromFile()

	logger.Info("Storage", "chromem 向量数据库初始化完成, 模型: %s, 已加载 %d 条文档记录", modelName, len(u.documentEntries))
	return nil
}

func (u *UnifiedDB) IsSQLInitialized() bool {
	return u != nil && u.sqlInitialized
}

func (u *UnifiedDB) IsVectorInitialized() bool {
	return u != nil && u.vectorInitialized
}

func (u *UnifiedDB) Close() error {
	if u.sqlDB != nil {
		if err := u.sqlDB.Close(); err != nil {
			return err
		}
	}
	return nil
}

func (u *UnifiedDB) Ping() error {
	if u.sqlDB == nil {
		return fmt.Errorf("SQL数据库未初始化")
	}
	_, err := u.sqlDB.Exec("SELECT 1")
	return err
}

// =============================================================================
// SQL 批量操作
// =============================================================================

func (u *UnifiedDB) ExecuteBatch(request DatabaseRequest) *BatchResult {
	startTime := time.Now()
	var results []OperationResult

	if len(request.Operations) == 0 {
		return &BatchResult{
			Success: false,
			Error:   "没有要执行的操作",
		}
	}

	var tx *sql.Tx
	var err error

	if request.Transaction && len(request.Operations) > 1 {
		tx, err = u.sqlDB.Begin()
		if err != nil {
			return &BatchResult{
				Success: false,
				Error:   fmt.Sprintf("开始事务失败: %v", err),
			}
		}
		defer func() {
			if err != nil {
				tx.Rollback()
			}
		}()
	}

	for i, op := range request.Operations {
		var result OperationResult

		switch opType := op.(type) {
		case map[string]any:
			opMap := opType
			operationType, _ := opMap["type"].(string)

			switch operationType {
			case "insert", "update", "delete", "select":
				result = u.executeDataOperation(opMap, tx)
			case "create", "drop", "truncate":
				result = u.executeTableOperation(opMap, tx)
			case "tables", "structure", "count":
				result = u.executeInfoOperation(opMap)
			default:
				result = OperationResult{
					Success:   false,
					Error:     fmt.Sprintf("未知的操作类型: %s", operationType),
					Operation: fmt.Sprintf("operation_%d", i),
				}
			}
		default:
			result = OperationResult{
				Success:   false,
				Error:     "无效的操作格式",
				Operation: fmt.Sprintf("operation_%d", i),
			}
		}

		results = append(results, result)

		if !result.Success && request.Transaction && tx != nil {
			tx.Rollback()
			return &BatchResult{
				Success:    false,
				Error:      fmt.Sprintf("操作 %d 失败，已回滚: %s", i, result.Error),
				Results:    results,
				TotalTime:  time.Since(startTime).Milliseconds(),
				Operations: len(request.Operations),
			}
		}
	}

	if request.Transaction && tx != nil {
		if err := tx.Commit(); err != nil {
			return &BatchResult{
				Success:    false,
				Error:      fmt.Sprintf("提交事务失败: %v", err),
				Results:    results,
				TotalTime:  time.Since(startTime).Milliseconds(),
				Operations: len(request.Operations),
			}
		}
	}

	return &BatchResult{
		Success:    true,
		Results:    results,
		TotalTime:  time.Since(startTime).Milliseconds(),
		Operations: len(request.Operations),
	}
}

func (u *UnifiedDB) executeDataOperation(op map[string]any, tx *sql.Tx) OperationResult {
	opType, _ := op["type"].(string)
	table, _ := op["table"].(string)

	if table == "" {
		return OperationResult{
			Success:   false,
			Error:     "表名不能为空",
			Operation: opType,
		}
	}

	table = sanitizeIdentifier(table)

	switch opType {
	case "insert":
		return u.executeInsert(table, op, tx)
	case "update":
		return u.executeUpdate(table, op, tx)
	case "delete":
		return u.executeDelete(table, op, tx)
	case "select":
		return u.executeSelect(table, op, tx)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的数据操作类型: %s", opType),
			Operation: opType,
			Table:     table,
		}
	}
}

func (u *UnifiedDB) executeInsert(table string, op map[string]any, tx *sql.Tx) OperationResult {
	data, _ := op["data"].(map[string]any)
	if len(data) == 0 {
		return OperationResult{
			Success:   false,
			Error:     "插入数据不能为空",
			Operation: "insert",
			Table:     table,
		}
	}

	var columns []string
	var placeholders []string
	var values []any

	for column, value := range data {
		columns = append(columns, sanitizeIdentifier(column))
		placeholders = append(placeholders, "?")
		values = append(values, value)
	}

	query := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query, values...)
	} else {
		result, err = u.sqlDB.Exec(query, values...)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("插入失败: %v", err),
			Operation: "insert",
			Table:     table,
		}
	}

	affectedRows, _ := result.RowsAffected()
	lastInsertID, _ := result.LastInsertId()

	return OperationResult{
		Success:      true,
		Operation:    "insert",
		Table:        table,
		AffectedRows: affectedRows,
		LastInsertID: lastInsertID,
	}
}

func (u *UnifiedDB) executeUpdate(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	data, _ := op["data"].(map[string]interface{})
	filter, _ := op["filter"].(map[string]interface{})

	if len(data) == 0 {
		return OperationResult{
			Success:   false,
			Error:     "更新数据不能为空",
			Operation: "update",
			Table:     table,
		}
	}

	var setClauses []string
	var values []interface{}

	for column, value := range data {
		setClauses = append(setClauses, fmt.Sprintf("`%s` = ?", sanitizeIdentifier(column)))
		values = append(values, value)
	}

	whereClause, whereValues := buildWhereClause(filter)
	if whereClause != "" {
		values = append(values, whereValues...)
	}

	query := fmt.Sprintf("UPDATE `%s` SET %s", table, strings.Join(setClauses, ", "))
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query, values...)
	} else {
		result, err = u.sqlDB.Exec(query, values...)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("更新失败: %v", err),
			Operation: "update",
			Table:     table,
		}
	}

	affectedRows, _ := result.RowsAffected()

	return OperationResult{
		Success:      true,
		Operation:    "update",
		Table:        table,
		AffectedRows: affectedRows,
	}
}

func (u *UnifiedDB) executeDelete(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	filter, _ := op["filter"].(map[string]interface{})

	whereClause, values := buildWhereClause(filter)

	query := fmt.Sprintf("DELETE FROM `%s`", table)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query, values...)
	} else {
		result, err = u.sqlDB.Exec(query, values...)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("删除失败: %v", err),
			Operation: "delete",
			Table:     table,
		}
	}

	affectedRows, _ := result.RowsAffected()

	return OperationResult{
		Success:      true,
		Operation:    "delete",
		Table:        table,
		AffectedRows: affectedRows,
	}
}

func (u *UnifiedDB) executeSelect(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	filter, _ := op["filter"].(map[string]interface{})
	limit, _ := op["limit"].(float64)
	offset, _ := op["offset"].(float64)
	order, _ := op["order"].([]interface{})

	query := fmt.Sprintf("SELECT * FROM `%s`", table)

	whereClause, values := buildWhereClause(filter)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	if len(order) > 0 {
		var orderClauses []string
		for _, o := range order {
			if orderMap, ok := o.(map[string]interface{}); ok {
				column, _ := orderMap["column"].(string)
				direction, _ := orderMap["direction"].(string)
				if column != "" {
					if direction == "" || (direction != "asc" && direction != "desc") {
						direction = "asc"
					}
					orderClauses = append(orderClauses,
						fmt.Sprintf("`%s` %s", sanitizeIdentifier(column), strings.ToUpper(direction)))
				}
			}
		}
		if len(orderClauses) > 0 {
			query += " ORDER BY " + strings.Join(orderClauses, ", ")
		}
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", int(limit))
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", int(offset))
		}
	}

	var rows *sql.Rows
	var err error

	if tx != nil {
		rows, err = tx.Query(query, values...)
	} else {
		rows, err = u.sqlDB.Query(query, values...)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("查询失败: %v", err),
			Operation: "select",
			Table:     table,
		}
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("获取列信息失败: %v", err),
			Operation: "select",
			Table:     table,
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
				Operation: "select",
				Table:     table,
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

	if err := rows.Err(); err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("遍历行数据时出错: %v", err),
			Operation: "select",
			Table:     table,
		}
	}

	return OperationResult{
		Success:   true,
		Operation: "select",
		Table:     table,
		Rows:      resultRows,
	}
}

func (u *UnifiedDB) executeTableOperation(op map[string]interface{}, tx *sql.Tx) OperationResult {
	opType, _ := op["type"].(string)
	table, _ := op["table"].(string)

	if table == "" && opType != "create" {
		return OperationResult{
			Success:   false,
			Error:     "表名不能为空",
			Operation: opType,
		}
	}

	if table != "" {
		table = sanitizeIdentifier(table)
	}

	switch opType {
	case "create":
		return u.executeCreateTable(op, tx)
	case "drop":
		return u.executeDropTable(table, tx)
	case "truncate":
		return u.executeTruncateTable(table, tx)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的表操作类型: %s", opType),
			Operation: opType,
			Table:     table,
		}
	}
}

func (u *UnifiedDB) executeCreateTable(op map[string]interface{}, tx *sql.Tx) OperationResult {
	definition, _ := op["definition"].(map[string]interface{})
	table, _ := op["table"].(string)

	if table == "" {
		return OperationResult{
			Success:   false,
			Error:     "表名不能为空",
			Operation: "create",
		}
	}

	table = sanitizeIdentifier(table)

	var createSQL strings.Builder
	createSQL.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (", table))

	columns, _ := definition["columns"].([]interface{})
	var columnDefs []string

	for _, col := range columns {
		if colMap, ok := col.(map[string]interface{}); ok {
			colName, _ := colMap["name"].(string)
			colType, _ := colMap["type"].(string)

			if colName == "" || colType == "" {
				continue
			}

			colDef := fmt.Sprintf("`%s` %s", sanitizeIdentifier(colName), colType)

			if primaryKey, _ := colMap["primary_key"].(bool); primaryKey {
				colDef += " PRIMARY KEY"
				if autoIncrement, _ := colMap["auto_increment"].(bool); autoIncrement {
					colDef += " AUTOINCREMENT"
				}
			}

			if notNull, _ := colMap["not_null"].(bool); notNull {
				colDef += " NOT NULL"
			}

			if unique, _ := colMap["unique"].(bool); unique {
				colDef += " UNIQUE"
			}

			if defaultValue := colMap["default"]; defaultValue != nil {
				switch v := defaultValue.(type) {
				case string:
					colDef += fmt.Sprintf(" DEFAULT '%s'", v)
				case float64, int:
					colDef += fmt.Sprintf(" DEFAULT %v", v)
				case bool:
					colDef += fmt.Sprintf(" DEFAULT %t", v)
				}
			}

			columnDefs = append(columnDefs, colDef)
		}
	}

	if len(columnDefs) == 0 {
		return OperationResult{
			Success:   false,
			Error:     "至少需要定义一个列",
			Operation: "create",
			Table:     table,
		}
	}

	createSQL.WriteString(strings.Join(columnDefs, ", "))

	indexes, _ := definition["indexes"].([]interface{})
	for _, idx := range indexes {
		if idxMap, ok := idx.(map[string]interface{}); ok {
			idxName, _ := idxMap["name"].(string)
			idxColumns, _ := idxMap["columns"].([]interface{})
			unique, _ := idxMap["unique"].(bool)

			if idxName == "" || len(idxColumns) == 0 {
				continue
			}

			var columnNames []string
			for _, col := range idxColumns {
				if colStr, ok := col.(string); ok {
					columnNames = append(columnNames, sanitizeIdentifier(colStr))
				}
			}

			if len(columnNames) > 0 {
				createSQL.WriteString(", ")
				if unique {
					createSQL.WriteString(fmt.Sprintf("UNIQUE INDEX `%s` ON `%s` (%s)", sanitizeIdentifier(idxName), table, strings.Join(columnNames, ", ")))
				} else {
					createSQL.WriteString(fmt.Sprintf("INDEX `%s` ON `%s` (%s)", sanitizeIdentifier(idxName), table, strings.Join(columnNames, ", ")))
				}
			}
		}
	}

	createSQL.WriteString(")")

	var err error
	if tx != nil {
		_, err = tx.Exec(createSQL.String())
	} else {
		_, err = u.sqlDB.Exec(createSQL.String())
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("创建表失败: %v", err),
			Operation: "create",
			Table:     table,
		}
	}

	return OperationResult{
		Success:   true,
		Operation: "create",
		Table:     table,
	}
}

func (u *UnifiedDB) executeDropTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)

	var err error
	if tx != nil {
		_, err = tx.Exec(query)
	} else {
		_, err = u.sqlDB.Exec(query)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("删除表失败: %v", err),
			Operation: "drop",
			Table:     table,
		}
	}

	return OperationResult{
		Success:   true,
		Operation: "drop",
		Table:     table,
	}
}

func (u *UnifiedDB) executeTruncateTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DELETE FROM `%s`", table)

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query)
	} else {
		result, err = u.sqlDB.Exec(query)
	}

	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("清空表失败: %v", err),
			Operation: "truncate",
			Table:     table,
		}
	}

	affectedRows, _ := result.RowsAffected()

	return OperationResult{
		Success:      true,
		Operation:    "truncate",
		Table:        table,
		AffectedRows: affectedRows,
	}
}

func (u *UnifiedDB) executeInfoOperation(op map[string]interface{}) OperationResult {
	opType, _ := op["type"].(string)
	table, _ := op["table"].(string)

	if table != "" {
		table = sanitizeIdentifier(table)
	}

	switch opType {
	case "tables":
		return u.executeGetTables()
	case "structure":
		if table == "" {
			return OperationResult{
				Success:   false,
				Error:     "表名不能为空",
				Operation: "structure",
			}
		}
		return u.executeGetTableStructure(table)
	case "count":
		if table == "" {
			return OperationResult{
				Success:   false,
				Error:     "表名不能为空",
				Operation: "count",
			}
		}
		return u.executeGetTableCount(table)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的信息操作类型: %s", opType),
			Operation: opType,
		}
	}
}

func (u *UnifiedDB) executeGetTables() OperationResult {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"

	rows, err := u.sqlDB.Query(query)
	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("获取表列表失败: %v", err),
			Operation: "tables",
		}
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return OperationResult{
				Success:   false,
				Error:     fmt.Sprintf("扫描表名失败: %v", err),
				Operation: "tables",
			}
		}
		tables = append(tables, table)
	}

	return OperationResult{
		Success:   true,
		Operation: "tables",
		Tables:    tables,
	}
}

func (u *UnifiedDB) executeGetTableStructure(table string) OperationResult {
	query := fmt.Sprintf("PRAGMA table_info(`%s`)", table)

	rows, err := u.sqlDB.Query(query)
	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("获取表结构失败: %v", err),
			Operation: "structure",
			Table:     table,
		}
	}
	defer rows.Close()

	var structure []map[string]interface{}
	for rows.Next() {
		var cid, name, typ, notnull, dflt_value, pk interface{}

		err = rows.Scan(&cid, &name, &typ, &notnull, &dflt_value, &pk)
		if err != nil {
			return OperationResult{
				Success:   false,
				Error:     fmt.Sprintf("扫描表结构失败: %v", err),
				Operation: "structure",
				Table:     table,
			}
		}

		nullStr := "YES"
		if notnull != nil {
			if notnullInt, ok := notnull.(int64); ok && notnullInt == 1 {
				nullStr = "NO"
			}
		}

		keyStr := ""
		if pk != nil {
			if pkInt, ok := pk.(int64); ok && pkInt == 1 {
				keyStr = "PRI"
			}
		}

		structure = append(structure, map[string]interface{}{
			"field":   name,
			"type":    typ,
			"null":    nullStr,
			"key":     keyStr,
			"default": dflt_value,
			"extra":   "",
		})
	}

	return OperationResult{
		Success:   true,
		Operation: "structure",
		Table:     table,
		Structure: structure,
	}
}

func (u *UnifiedDB) executeGetTableCount(table string) OperationResult {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)

	row := u.sqlDB.QueryRow(query)

	var count int64
	if err := row.Scan(&count); err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("获取表记录数失败: %v", err),
			Operation: "count",
			Table:     table,
		}
	}

	return OperationResult{
		Success:   true,
		Operation: "count",
		Table:     table,
		Count:     count,
	}
}

// =============================================================================
// 向量数据库操作
// =============================================================================

func (u *UnifiedDB) VectorAddMessage(ctx context.Context, role string, content string) (string, error) {
	if u.collection == nil {
		return "", fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	u.messageIDCounter++
	id := fmt.Sprintf("msg-%d", u.messageIDCounter)

	metadata := map[string]string{
		"role": role,
	}

	doc := chromem.Document{
		ID:       id,
		Metadata: metadata,
		Content:  content,
	}

	err := u.collection.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
	if err != nil {
		return "", fmt.Errorf("chromem 添加消息失败: %v", err)
	}

	u.documentEntriesMu.Lock()
	u.documentEntries = append(u.documentEntries, DocumentEntry{ID: id, Role: role, Content: content})
	u.documentEntriesMu.Unlock()

	u.saveEntriesToFile()

	return id, nil
}

func (u *UnifiedDB) VectorAddMessageSilent(ctx context.Context, role string, content string) error {
	if u.collection == nil {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	if strings.TrimSpace(content) == "" {
		return nil
	}

	u.messageIDCounter++
	id := fmt.Sprintf("msg-%d", u.messageIDCounter)

	metadata := map[string]string{
		"role": role,
	}

	doc := chromem.Document{
		ID:       id,
		Metadata: metadata,
		Content:  content,
	}

	err := u.collection.AddDocuments(ctx, []chromem.Document{doc}, runtime.NumCPU())
	if err != nil {
		return fmt.Errorf("chromem 添加消息失败: %v", err)
	}

	u.documentEntriesMu.Lock()
	u.documentEntries = append(u.documentEntries, DocumentEntry{ID: id, Role: role, Content: content})
	u.documentEntriesMu.Unlock()

	u.saveEntriesToFile()

	return nil
}

func (u *UnifiedDB) VectorQueryMessages(ctx context.Context, queryText string, topK int) ([]string, error) {
	if u.collection == nil {
		return nil, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	if topK <= 0 {
		topK = 10
	}

	docCount := u.collection.Count()
	if topK > docCount {
		topK = docCount
	}
	if topK == 0 {
		return []string{}, nil
	}

	results, err := u.collection.Query(ctx, queryText, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem 查询消息失败: %v", err)
	}

	// chromem-go 已按相似度降序返回结果，此处保留原始顺序
	messages := make([]string, 0, len(results))
	for _, result := range results {
		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}

		msg := chromemMessage{Role: role, Content: result.Content}
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		messages = append(messages, string(jsonBytes))
	}

	return messages, nil
}

func (u *UnifiedDB) VectorQueryMessagesWithContent(ctx context.Context, queryText string, topK int) ([]VectorQueryResult, error) {
	if u.collection == nil {
		return nil, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	if topK <= 0 {
		topK = 10
	}

	docCount := u.collection.Count()
	if topK > docCount {
		topK = docCount
	}
	if topK == 0 {
		return []VectorQueryResult{}, nil
	}

	results, err := u.collection.Query(ctx, queryText, topK, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem 查询消息失败: %v", err)
	}

	// chromem-go 已按相似度降序返回结果，此处保留原始顺序
	messages := make([]VectorQueryResult, 0, len(results))
	for _, result := range results {
		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}
		messages = append(messages, VectorQueryResult{
			ID:         result.ID,
			Role:       role,
			Content:    result.Content,
			Similarity: result.Similarity,
		})
	}

	return messages, nil
}

func (u *UnifiedDB) VectorDeleteMessage(ctx context.Context, id string) error {
	if u.collection == nil {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	if err := u.collection.Delete(ctx, nil, nil, id); err != nil {
		return fmt.Errorf("chromem 删除消息失败: %v", err)
	}

	u.documentEntriesMu.Lock()
	for i, entry := range u.documentEntries {
		if entry.ID == id {
			u.documentEntries = append(u.documentEntries[:i], u.documentEntries[i+1:]...)
			break
		}
	}
	u.documentEntriesMu.Unlock()

	u.saveEntriesToFile()

	return nil
}

func (u *UnifiedDB) VectorGetCollectionCount() int {
	if u.collection == nil {
		return 0
	}
	return u.collection.Count()
}

func (u *UnifiedDB) VectorGetDocuments(offset int, limit int) ([]DocumentEntry, int) {
	u.documentEntriesMu.RLock()
	defer u.documentEntriesMu.RUnlock()

	total := len(u.documentEntries)

	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []DocumentEntry{}, total
	}

	end := offset + limit
	if end > total {
		end = total
	}

	entries := make([]DocumentEntry, end-offset)
	copy(entries, u.documentEntries[offset:end])
	return entries, total
}

func (u *UnifiedDB) VectorGetEntryCount() int {
	u.documentEntriesMu.RLock()
	defer u.documentEntriesMu.RUnlock()
	return len(u.documentEntries)
}

func (u *UnifiedDB) VectorHasSyncMismatch() bool {
	if u.collection == nil {
		return false
	}
	return u.collection.Count() != u.VectorGetEntryCount()
}

func (u *UnifiedDB) VectorRebuildEntries(ctx context.Context) (int, error) {
	if u.collection == nil {
		return 0, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInit")
	}

	chromemCount := u.collection.Count()
	if chromemCount == 0 {
		u.documentEntriesMu.Lock()
		u.documentEntries = nil
		u.documentEntriesMu.Unlock()
		u.saveEntriesToFile()
		return 0, nil
	}

	results, err := u.collection.Query(ctx, " ", chromemCount, nil, nil)
	if err != nil {
		return 0, fmt.Errorf("chromem 查询所有文档失败: %v", err)
	}

	seenIDs := make(map[string]bool)
	var newEntries []DocumentEntry

	for _, result := range results {
		if seenIDs[result.ID] {
			continue
		}
		seenIDs[result.ID] = true

		role := "user"
		if r, ok := result.Metadata["role"]; ok {
			role = r
		}

		newEntries = append(newEntries, DocumentEntry{
			ID:      result.ID,
			Role:    role,
			Content: result.Content,
		})
	}

	u.documentEntriesMu.Lock()
	u.documentEntries = newEntries
	u.documentEntriesMu.Unlock()

	u.saveEntriesToFile()

	maxNum := 0
	for _, entry := range newEntries {
		var num int
		if _, scanErr := fmt.Sscanf(entry.ID, "msg-%d", &num); scanErr == nil && num > maxNum {
			maxNum = num
		}
	}
	if maxNum > u.messageIDCounter {
		u.messageIDCounter = maxNum
	}

	logger.Info("Storage", "chromem 重建 entries 完成, 共 %d 条文档", len(newEntries))

	return len(newEntries), nil
}

func (u *UnifiedDB) loadEntriesFromFile() {
	data, err := os.ReadFile(u.entriesFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "chromem 读取 entries.json 失败: %v", err)
		}
		return
	}

	if len(data) == 0 {
		return
	}

	var entries []DocumentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.Warn("Storage", "chromem entries.json 解析失败: %v", err)
		return
	}

	u.documentEntriesMu.Lock()
	u.documentEntries = entries
	u.documentEntriesMu.Unlock()

	maxNum := 0
	for _, entry := range entries {
		var num int
		if _, scanErr := fmt.Sscanf(entry.ID, "msg-%d", &num); scanErr == nil && num > maxNum {
			maxNum = num
		}
	}
	if maxNum > u.messageIDCounter {
		u.messageIDCounter = maxNum
	}

	logger.Info("Storage", "chromem 从 entries.json 加载了 %d 条文档, ID计数器重置为 %d", len(entries), u.messageIDCounter)
}

func (u *UnifiedDB) saveEntriesToFile() {
	u.documentEntriesMu.RLock()
	data, err := json.MarshalIndent(u.documentEntries, "", "  ")
	u.documentEntriesMu.RUnlock()
	if err != nil {
		logger.Error("Storage", "chromem entries 序列化失败: %v", err)
		return
	}

	if err := os.WriteFile(u.entriesFilePath, data, 0644); err != nil {
		logger.Error("Storage", "chromem entries.json 写入失败: %v", err)
	}
}

// =============================================================================
// 全局包装函数 — 保持与原有代码的兼容性
// =============================================================================

func EnsureDBInitialized() error {
	if Unified == nil || !Unified.IsSQLInitialized() {
		if err := InitUnifiedDB(*config.SQLDBPath, *config.VectorDBDir); err != nil {
			return err
		}
	}
	if err := Unified.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %v", err)
	}
	return nil
}

func ExecuteDatabaseRequest(request DatabaseRequest) *BatchResult {
	if err := EnsureDBInitialized(); err != nil {
		return &BatchResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	return Unified.ExecuteBatch(request)
}

func IsInitialized() bool {
	return Unified != nil && Unified.IsVectorInitialized()
}

func AddMessage(ctx context.Context, role string, content string) error {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return fmt.Errorf("chromem 未初始化, 请先调用 VectorInit")
	}
	return Unified.VectorAddMessageSilent(ctx, role, content)
}

func AddMessageWithID(ctx context.Context, role string, content string) (string, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return "", fmt.Errorf("chromem 未初始化, 请先调用 VectorInit")
	}
	return Unified.VectorAddMessage(ctx, role, content)
}

func QueryMessagesWithContent(ctx context.Context, queryText string, topK int) ([]VectorQueryResult, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return nil, fmt.Errorf("chromem 未初始化, 请先调用 VectorInit")
	}
	return Unified.VectorQueryMessagesWithContent(ctx, queryText, topK)
}

func DeleteMessage(ctx context.Context, id string) error {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return fmt.Errorf("chromem 未初始化, 请先调用 VectorInit")
	}
	return Unified.VectorDeleteMessage(ctx, id)
}

func GetCollectionCount() int {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0
	}
	return Unified.VectorGetCollectionCount()
}

func GetDocuments(offset int, limit int) ([]DocumentEntry, int) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return []DocumentEntry{}, 0
	}
	return Unified.VectorGetDocuments(offset, limit)
}

func GetEntryCount() int {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0
	}
	return Unified.VectorGetEntryCount()
}

func HasSyncMismatch() bool {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return false
	}
	return Unified.VectorHasSyncMismatch()
}

func RebuildEntries(ctx context.Context) (int, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0, fmt.Errorf("chromem 未初始化, 请先调用 VectorInit")
	}
	return Unified.VectorRebuildEntries(ctx)
}

func Init(baseURL string, apiKey string, modelName string) error {
	if Unified == nil {
		if err := InitUnifiedDB(*config.SQLDBPath, *config.VectorDBDir); err != nil {
			return err
		}
	}
	return Unified.VectorInit(baseURL, apiKey, modelName)
}

// =============================================================================
// 工具函数
// =============================================================================

func buildWhereClause(filter map[string]interface{}) (string, []interface{}) {
	if len(filter) == 0 {
		return "", nil
	}

	var conditions []string
	var values []interface{}

	for key, value := range filter {
		if opMap, ok := value.(map[string]interface{}); ok {
			for op, opValue := range opMap {
				switch op {
				case "$eq":
					conditions = append(conditions, fmt.Sprintf("`%s` = ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$ne":
					conditions = append(conditions, fmt.Sprintf("`%s` != ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$gt":
					conditions = append(conditions, fmt.Sprintf("`%s` > ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$gte":
					conditions = append(conditions, fmt.Sprintf("`%s` >= ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$lt":
					conditions = append(conditions, fmt.Sprintf("`%s` < ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$lte":
					conditions = append(conditions, fmt.Sprintf("`%s` <= ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$like":
					conditions = append(conditions, fmt.Sprintf("`%s` LIKE ?", sanitizeIdentifier(key)))
					values = append(values, opValue)
				case "$in":
					if arr, ok := opValue.([]interface{}); ok {
						placeholders := make([]string, len(arr))
						for i := range arr {
							placeholders[i] = "?"
							values = append(values, arr[i])
						}
						conditions = append(conditions,
							fmt.Sprintf("`%s` IN (%s)", sanitizeIdentifier(key), strings.Join(placeholders, ", ")))
					}
				}
			}
		} else {
			conditions = append(conditions, fmt.Sprintf("`%s` = ?", sanitizeIdentifier(key)))
			values = append(values, value)
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return strings.Join(conditions, " AND "), values
}

func sanitizeIdentifier(name string) string {
	dangerousChars := []string{";", "'", "\"", "\\", "--", "/*", "*/", "(", ")", "[", "]"}
	for _, char := range dangerousChars {
		name = strings.ReplaceAll(name, char, "")
	}

	sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
		"TRUNCATE", "EXEC", "UNION", "JOIN", "WHERE", "FROM", "SET"}
	for _, keyword := range sqlKeywords {
		if strings.ToUpper(name) == keyword {
			return "_" + name
		}
	}

	return name
}
