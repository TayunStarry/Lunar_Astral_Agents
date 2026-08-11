package module

import (
	"LunarSubsystem/general_config"
	"LunarSubsystem/general_logger"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func init() {
	logger.SetDevMode(*config.Developer, "local_data/documents/debug")
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
	logger.Info("Storage", "知识库初始化完成: %s", dbPath)
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
// 知识库批量操作
// =============================================================================

// ExecuteBatch 批量执行知识库操作，支持事务包裹
func (d *KnowledgeDB) ExecuteBatch(request KnowledgeRequest) *BatchResult {
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
		tx, err = d.knowledgeDB.Begin()
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
				result = d.executeDataOperation(opMap, tx)
			case "create", "drop", "truncate":
				result = d.executeTableOperation(opMap, tx)
			case "tables", "structure", "count":
				result = d.executeInfoOperation(opMap)
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

func (d *KnowledgeDB) executeDataOperation(op map[string]any, tx *sql.Tx) OperationResult {
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
		return d.executeInsert(table, op, tx)
	case "update":
		return d.executeUpdate(table, op, tx)
	case "delete":
		return d.executeDelete(table, op, tx)
	case "select":
		return d.executeSelect(table, op, tx)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的数据操作类型: %s", opType),
			Operation: opType,
			Table:     table,
		}
	}
}

func (d *KnowledgeDB) executeInsert(table string, op map[string]any, tx *sql.Tx) OperationResult {
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
		result, err = d.knowledgeDB.Exec(query, values...)
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

func (d *KnowledgeDB) executeUpdate(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
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
		result, err = d.knowledgeDB.Exec(query, values...)
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

func (d *KnowledgeDB) executeDelete(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
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
		result, err = d.knowledgeDB.Exec(query, values...)
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

func (d *KnowledgeDB) executeSelect(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
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
		rows, err = d.knowledgeDB.Query(query, values...)
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

func (d *KnowledgeDB) executeTableOperation(op map[string]interface{}, tx *sql.Tx) OperationResult {
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
		return d.executeCreateTable(op, tx)
	case "drop":
		return d.executeDropTable(table, tx)
	case "truncate":
		return d.executeTruncateTable(table, tx)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的表操作类型: %s", opType),
			Operation: opType,
			Table:     table,
		}
	}
}

func (d *KnowledgeDB) executeCreateTable(op map[string]interface{}, tx *sql.Tx) OperationResult {
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
		_, err = d.knowledgeDB.Exec(createSQL.String())
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

func (d *KnowledgeDB) executeDropTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)

	var err error
	if tx != nil {
		_, err = tx.Exec(query)
	} else {
		_, err = d.knowledgeDB.Exec(query)
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

func (d *KnowledgeDB) executeTruncateTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DELETE FROM `%s`", table)

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query)
	} else {
		result, err = d.knowledgeDB.Exec(query)
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

func (d *KnowledgeDB) executeInfoOperation(op map[string]interface{}) OperationResult {
	opType, _ := op["type"].(string)
	table, _ := op["table"].(string)

	if table != "" {
		table = sanitizeIdentifier(table)
	}

	switch opType {
	case "tables":
		return d.executeGetTables()
	case "structure":
		if table == "" {
			return OperationResult{
				Success:   false,
				Error:     "表名不能为空",
				Operation: "structure",
			}
		}
		return d.executeGetTableStructure(table)
	case "count":
		if table == "" {
			return OperationResult{
				Success:   false,
				Error:     "表名不能为空",
				Operation: "count",
			}
		}
		return d.executeGetTableCount(table)
	default:
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("不支持的信息操作类型: %s", opType),
			Operation: opType,
		}
	}
}

func (d *KnowledgeDB) executeGetTables() OperationResult {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"

	rows, err := d.knowledgeDB.Query(query)
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

func (d *KnowledgeDB) executeGetTableStructure(table string) OperationResult {
	query := fmt.Sprintf("PRAGMA table_info(`%s`)", table)

	rows, err := d.knowledgeDB.Query(query)
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

func (d *KnowledgeDB) executeGetTableCount(table string) OperationResult {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)

	row := d.knowledgeDB.QueryRow(query)

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
// 全局包装函数 — 知识库
// =============================================================================

// EnsureKnowledgeInitialized 确保知识库已初始化并连接可用
func EnsureKnowledgeInitialized() error {
	if KnowledgeDatabase == nil || !KnowledgeDatabase.IsKnowledgeInitialized() {
		if err := InitKnowledgeDB(*config.KnowledgeDBPath); err != nil {
			return err
		}
	}
	if err := KnowledgeDatabase.Ping(); err != nil {
		return fmt.Errorf("知识库连接失败: %v", err)
	}
	return nil
}

// ExecuteKnowledgeRequest 全局包装 — 执行知识库批量请求
func ExecuteKnowledgeRequest(request KnowledgeRequest) *BatchResult {
	if err := EnsureKnowledgeInitialized(); err != nil {
		return &BatchResult{
			Success: false,
			Error:   err.Error(),
		}
	}
	return KnowledgeDatabase.ExecuteBatch(request)
}

// =============================================================================
// 知识库工具函数
// =============================================================================

// buildWhereClause 根据 MongoDB 风格的 filter 构造 SQL WHERE 子句与参数列表
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

// sanitizeIdentifier 过滤 SQL 标识符中的危险字符与保留关键字
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
