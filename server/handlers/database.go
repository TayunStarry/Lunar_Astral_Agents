package handlers

import (
	"Lunar-Astral-Agents/server/config"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database 数据库封装结构
type Database struct {
	db *sql.DB
}

// OperationResult 单个操作结果
type OperationResult struct {
	Success      bool                     `json:"success"`
	Error        string                   `json:"error,omitempty"`
	Operation    string                   `json:"operation"`
	Rows         []map[string]interface{} `json:"rows,omitempty"`
	AffectedRows int64                    `json:"affected_rows,omitempty"`
	LastInsertID int64                    `json:"last_insert_id,omitempty"`
	Table        string                   `json:"table,omitempty"`
	Structure    []map[string]interface{} `json:"structure,omitempty"`
	Tables       []string                 `json:"tables,omitempty"`
	Count        int64                    `json:"count,omitempty"`
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

// NewDatabase 创建新的数据库实例
func NewDatabase() (*Database, error) {
	db, err := initSQLite(*config.Database)
	if err != nil {
		return nil, fmt.Errorf("初始化SQLite数据库失败: %v", err)
	}

	log.Printf("SQLite数据库连接成功: %s", *config.Database)

	return &Database{
		db: db,
	}, nil
}

// initSQLite 初始化SQLite数据库
func initSQLite(dbPath string) (*sql.DB, error) {
	// 确保目录存在
	dir := filepath.Dir(dbPath)
	if dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("创建数据库目录失败: %v", err)
		}
	}

	// 连接SQLite数据库
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("连接SQLite失败: %v", err)
	}

	// 设置连接池参数
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 测试连接
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("测试SQLite连接失败: %v", err)
	}

	// 设置优化选项
	_, err = db.Exec("PRAGMA synchronous = NORMAL")
	if err != nil {
		log.Printf("设置SQLite同步模式失败: %v", err)
	}

	_, err = db.Exec("PRAGMA cache_size = 10000")
	if err != nil {
		log.Printf("设置SQLite缓存大小失败: %v", err)
	}

	// 启用外键约束
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	if err != nil {
		log.Printf("启用外键约束失败: %v", err)
	}

	return db, nil
}

// Close 关闭数据库连接
func (d *Database) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

// Ping 检查数据库连接状态
func (d *Database) Ping() error {
	_, err := d.db.Exec("SELECT 1")
	return err
}

// ExecuteBatch 执行批量操作
func (d *Database) ExecuteBatch(request DatabaseRequest) *BatchResult {
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

	// 如果启用事务，开始事务
	if request.Transaction && len(request.Operations) > 1 {
		tx, err = d.db.Begin()
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

	// 执行每个操作
	for i, op := range request.Operations {
		var result OperationResult

		// 根据操作类型执行不同的操作
		switch opType := op.(type) {
		case map[string]interface{}:
			// 解析操作类型
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

		// 如果操作失败且启用了事务，回滚
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

	// 提交事务
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

// executeDataOperation 执行数据操作
func (d *Database) executeDataOperation(op map[string]interface{}, tx *sql.Tx) OperationResult {
	opType, _ := op["type"].(string)
	table, _ := op["table"].(string)

	if table == "" {
		return OperationResult{
			Success:   false,
			Error:     "表名不能为空",
			Operation: opType,
		}
	}

	// 安全过滤表名
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

// executeInsert 执行插入操作
func (d *Database) executeInsert(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	data, _ := op["data"].(map[string]interface{})
	if len(data) == 0 {
		return OperationResult{
			Success:   false,
			Error:     "插入数据不能为空",
			Operation: "insert",
			Table:     table,
		}
	}

	// 构建INSERT语句
	var columns []string
	var placeholders []string
	var values []interface{}

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
		result, err = d.db.Exec(query, values...)
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

// executeUpdate 执行更新操作
func (d *Database) executeUpdate(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
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

	// 构建SET子句
	var setClauses []string
	var values []interface{}

	for column, value := range data {
		setClauses = append(setClauses, fmt.Sprintf("`%s` = ?", sanitizeIdentifier(column)))
		values = append(values, value)
	}

	// 构建WHERE子句
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
		result, err = d.db.Exec(query, values...)
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

// executeDelete 执行删除操作
func (d *Database) executeDelete(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	filter, _ := op["filter"].(map[string]interface{})

	// 构建WHERE子句
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
		result, err = d.db.Exec(query, values...)
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

// executeSelect 执行查询操作
func (d *Database) executeSelect(table string, op map[string]interface{}, tx *sql.Tx) OperationResult {
	filter, _ := op["filter"].(map[string]interface{})
	limit, _ := op["limit"].(float64)
	offset, _ := op["offset"].(float64)
	order, _ := op["order"].([]interface{})

	// 构建查询语句
	query := fmt.Sprintf("SELECT * FROM `%s`", table)

	// WHERE子句
	whereClause, values := buildWhereClause(filter)
	if whereClause != "" {
		query += " WHERE " + whereClause
	}

	// ORDER BY子句
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

	// LIMIT和OFFSET
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", int(limit))
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", int(offset))
		}
	}

	// 执行查询
	var rows *sql.Rows
	var err error

	if tx != nil {
		rows, err = tx.Query(query, values...)
	} else {
		rows, err = d.db.Query(query, values...)
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

	// 获取列信息
	columns, err := rows.Columns()
	if err != nil {
		return OperationResult{
			Success:   false,
			Error:     fmt.Sprintf("获取列信息失败: %v", err),
			Operation: "select",
			Table:     table,
		}
	}

	// 读取数据
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

// executeTableOperation 执行表操作
func (d *Database) executeTableOperation(op map[string]interface{}, tx *sql.Tx) OperationResult {
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

// executeCreateTable 执行创建表操作
func (d *Database) executeCreateTable(op map[string]interface{}, tx *sql.Tx) OperationResult {
	// 解析TableDefinition
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

	// 构建CREATE TABLE语句
	var createSQL strings.Builder
	createSQL.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (", table))

	// 解析列定义
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

			// 处理约束
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

	// 解析索引定义
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

	// 执行创建表语句
	var err error
	if tx != nil {
		_, err = tx.Exec(createSQL.String())
	} else {
		_, err = d.db.Exec(createSQL.String())
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

// executeDropTable 执行删除表操作
func (d *Database) executeDropTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)

	var err error
	if tx != nil {
		_, err = tx.Exec(query)
	} else {
		_, err = d.db.Exec(query)
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

// executeTruncateTable 执行清空表操作
func (d *Database) executeTruncateTable(table string, tx *sql.Tx) OperationResult {
	query := fmt.Sprintf("DELETE FROM `%s`", table)

	var result sql.Result
	var err error

	if tx != nil {
		result, err = tx.Exec(query)
	} else {
		result, err = d.db.Exec(query)
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

// executeInfoOperation 执行信息操作
func (d *Database) executeInfoOperation(op map[string]interface{}) OperationResult {
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

// executeGetTables 执行获取表列表
func (d *Database) executeGetTables() OperationResult {
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'"

	rows, err := d.db.Query(query)
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

// executeGetTableStructure 执行获取表结构
func (d *Database) executeGetTableStructure(table string) OperationResult {
	query := fmt.Sprintf("PRAGMA table_info(`%s`)", table)

	rows, err := d.db.Query(query)
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

// executeGetTableCount 执行获取表记录数
func (d *Database) executeGetTableCount(table string) OperationResult {
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)

	row := d.db.QueryRow(query)

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

// buildWhereClause 构建WHERE子句
func buildWhereClause(filter map[string]interface{}) (string, []interface{}) {
	if len(filter) == 0 {
		return "", nil
	}

	var conditions []string
	var values []interface{}

	for key, value := range filter {
		// 支持多种比较操作符
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
			// 默认使用等于
			conditions = append(conditions, fmt.Sprintf("`%s` = ?", sanitizeIdentifier(key)))
			values = append(values, value)
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return strings.Join(conditions, " AND "), values
}

// sanitizeIdentifier 安全过滤标识符（表名、列名）
func sanitizeIdentifier(name string) string {
	// 移除危险字符
	dangerousChars := []string{";", "'", "\"", "\\", "--", "/*", "*/", "(", ")", "[", "]"}
	for _, char := range dangerousChars {
		name = strings.ReplaceAll(name, char, "")
	}

	// 移除SQL关键字（简单检查）
	sqlKeywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER",
		"TRUNCATE", "EXEC", "UNION", "JOIN", "WHERE", "FROM", "SET"}
	for _, keyword := range sqlKeywords {
		if strings.ToUpper(name) == keyword {
			return "_" + name
		}
	}

	return name
}

// 全局数据库实例
var dbInstance *Database

// initDatabase 初始化全局数据库实例
func initDatabase() {
	var err error
	dbInstance, err = NewDatabase()
	if err != nil {
		log.Printf("初始化全局数据库实例失败: %v", err)
	}
}

// 确保数据库实例初始化
func ensureDatabaseInitialized() error {
	if dbInstance == nil {
		initDatabase()
		if dbInstance == nil {
			return fmt.Errorf("数据库未初始化")
		}
	}
	// 检查数据库连接状态
	if err := dbInstance.Ping(); err != nil {
		return fmt.Errorf("数据库连接失败: %v", err)
	}
	return nil
}

// DatabaseHandler 统一的数据库处理器
func DatabaseHandler(w http.ResponseWriter, r *http.Request) {
	// 检查请求方法
	if r.Method != "POST" {
		http.Error(w, "数据库请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	// 确保数据库初始化
	if err := ensureDatabaseInitialized(); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> %v", err), http.StatusInternalServerError)
		return
	}

	// 解析请求体
	var req DatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> 解析请求失败: %v", err), http.StatusBadRequest)
		return
	}

	// 执行批量操作
	result := dbInstance.ExecuteBatch(req)

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 返回结果
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, fmt.Sprintf("数据库请求[ERROR] -> 编码响应失败: %v", err), http.StatusInternalServerError)
		return
	}

	// 记录日志
	if *config.DevMode {
		log.Printf("数据库批量操作成功，执行 %d 个操作，耗时 %dms",
			result.Operations, result.TotalTime)
	}
}
