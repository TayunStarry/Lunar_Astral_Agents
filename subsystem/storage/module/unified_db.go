package module

import (
	"config"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"logger"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	u.collectionsDir = filepath.Join(vectorDir, "collections")
	if err := os.MkdirAll(u.collectionsDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %v", err)
	}
	u.collections = make(map[string]*Collection)

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

// VectorInitInstance 初始化向量数据库实例（不创建任何集合）
// 仅配置嵌入服务连接，并加载已存在的集合到内存
func (u *UnifiedDB) VectorInitInstance(baseURL string, apiKey string) error {
	if u.vectorInitialized {
		return nil
	}

	if u.collectionsDir == "" {
		return fmt.Errorf("向量数据库未配置存储路径, 请先调用 InitUnifiedDB")
	}

	u.embeddingBaseURL = baseURL
	u.embeddingAPIKey = apiKey
	u.httpClient = &http.Client{Timeout: 120 * time.Second}
	u.vectorInitialized = true

	u.loadAllCollections()

	logger.Info("Storage", "向量数据库实例初始化完成, base_url: %s, 已加载 %d 个集合",
		u.embeddingBaseURL, len(u.collections))
	return nil
}

// validateCollectionName 校验集合名合法性（仅字母数字下划线连字符，防路径穿越）
func validateCollectionName(name string) error {
	if name == "" {
		return fmt.Errorf("集合名不能为空")
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("集合名仅允许字母、数字、下划线、连字符: %s", name)
		}
	}
	return nil
}

// CollectionInit 创建或打开指定名称的集合
// 通过探针文本嵌入一次确定向量维度，写入 metadata.json
// 若集合已存在且 model 一致则直接返回，model 变更则重新探针并更新维度
func (u *UnifiedDB) CollectionInit(ctx context.Context, name string, modelName string) error {
	if !u.vectorInitialized {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	if err := validateCollectionName(name); err != nil {
		return err
	}

	// 已存在则直接返回
	u.collectionsMu.RLock()
	if _, ok := u.collections[name]; ok {
		u.collectionsMu.RUnlock()
		return nil
	}
	u.collectionsMu.RUnlock()

	collDir := filepath.Join(u.collectionsDir, name)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("创建集合目录失败: %v", err)
	}

	filePath := filepath.Join(collDir, "documents.json")
	metaPath := filepath.Join(collDir, "metadata.json")

	// 尝试加载已有 metadata
	var meta collectionMeta
	if data, err := os.ReadFile(metaPath); err == nil && len(data) > 0 {
		if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
			return fmt.Errorf("metadata.json 解析失败: %v", jsonErr)
		}
	}

	// metadata 不存在或 model 变更时，重新探针定维度
	if meta.Dimension == 0 || meta.Model != modelName {
		probeVec, err := u.embedText(ctx, modelName, name)
		if err != nil {
			return fmt.Errorf("探针文本嵌入失败: %v", err)
		}
		meta.Model = modelName
		meta.Dimension = len(probeVec)
		if err := saveCollectionMeta(metaPath, meta); err != nil {
			return fmt.Errorf("写入 metadata.json 失败: %v", err)
		}
	}

	c := &Collection{
		Name:      name,
		Model:     meta.Model,
		Dimension: meta.Dimension,
		Documents: make([]VectorDocument, 0),
		filePath:  filePath,
		metaPath:  metaPath,
	}
	c.loadDocumentsFromFile()

	u.collectionsMu.Lock()
	u.collections[name] = c
	u.collectionsMu.Unlock()

	logger.Info("Storage", "集合 [%s] 初始化完成, 模型: %s, 维度: %d, 文档数: %d",
		name, c.Model, c.Dimension, len(c.Documents))
	return nil
}

// saveCollectionMeta 写入集合元数据
func saveCollectionMeta(metaPath string, meta collectionMeta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0644)
}

// getCollection 获取集合实例，不存在返回错误
func (u *UnifiedDB) getCollection(name string) (*Collection, error) {
	u.collectionsMu.RLock()
	defer u.collectionsMu.RUnlock()
	c, ok := u.collections[name]
	if !ok {
		return nil, fmt.Errorf("集合 [%s] 不存在, 请先调用 CollectionInit", name)
	}
	return c, nil
}

// loadAllCollections 启动时扫描 collectionsDir 加载所有集合到内存
func (u *UnifiedDB) loadAllCollections() {
	entries, err := os.ReadDir(u.collectionsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "扫描集合目录失败: %v", err)
		}
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if validateCollectionName(name) != nil {
			continue
		}
		collDir := filepath.Join(u.collectionsDir, name)
		metaPath := filepath.Join(collDir, "metadata.json")
		filePath := filepath.Join(collDir, "documents.json")

		var meta collectionMeta
		if data, err := os.ReadFile(metaPath); err == nil && len(data) > 0 {
			if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
				logger.Warn("Storage", "集合 [%s] metadata 解析失败: %v", name, jsonErr)
				continue
			}
		}

		if meta.Model == "" || meta.Dimension == 0 {
			logger.Warn("Storage", "集合 [%s] metadata 不完整, 跳过加载", name)
			continue
		}

		c := &Collection{
			Name:      name,
			Model:     meta.Model,
			Dimension: meta.Dimension,
			Documents: make([]VectorDocument, 0),
			filePath:  filePath,
			metaPath:  metaPath,
		}
		c.loadDocumentsFromFile()

		u.collectionsMu.Lock()
		u.collections[name] = c
		u.collectionsMu.Unlock()

		logger.Info("Storage", "已加载集合 [%s], 模型: %s, 维度: %d, 文档数: %d",
			name, c.Model, c.Dimension, len(c.Documents))
	}
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
// 向量数据库操作（多集合）
// =============================================================================

func (u *UnifiedDB) VectorAddMessage(ctx context.Context, collectionName string, role string, content string) (string, error) {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("消息内容不能为空")
	}

	embedding, err := u.embedText(ctx, c.Model, content)
	if err != nil {
		return "", fmt.Errorf("嵌入文本失败: %v", err)
	}

	if len(embedding) != c.Dimension {
		return "", fmt.Errorf("嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(embedding), collectionName, c.Dimension)
	}

	c.mu.Lock()
	c.idCounter++
	id := fmt.Sprintf("msg-%d", c.idCounter)
	c.Documents = append(c.Documents, VectorDocument{
		ID:        id,
		Role:      role,
		Content:   content,
		Embedding: embedding,
	})
	c.mu.Unlock()

	c.saveDocumentsToFile()
	return id, nil
}

func (u *UnifiedDB) VectorAddMessageSilent(ctx context.Context, collectionName string, role string, content string) error {
	_, err := u.VectorAddMessage(ctx, collectionName, role, content)
	return err
}

func (u *UnifiedDB) VectorQueryMessages(ctx context.Context, collectionName string, queryText string, topK int) ([]string, error) {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = 10
	}

	queryVec, err := u.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("嵌入查询文本失败: %v", err)
	}

	if len(queryVec) != c.Dimension {
		return nil, fmt.Errorf("查询嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(queryVec), collectionName, c.Dimension)
	}

	results := c.queryTopK(queryVec, topK)

	messages := make([]string, 0, len(results))
	for _, r := range results {
		msg := chromemMessage{Role: r.Role, Content: r.Content}
		jsonBytes, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		messages = append(messages, string(jsonBytes))
	}
	return messages, nil
}

func (u *UnifiedDB) VectorQueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]VectorQueryResult, error) {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return nil, err
	}

	if topK <= 0 {
		topK = 10
	}

	queryVec, err := u.embedText(ctx, c.Model, queryText)
	if err != nil {
		return nil, fmt.Errorf("嵌入查询文本失败: %v", err)
	}

	if len(queryVec) != c.Dimension {
		return nil, fmt.Errorf("查询嵌入维度 %d 与集合 [%s] 维度 %d 不符",
			len(queryVec), collectionName, c.Dimension)
	}

	return c.queryTopK(queryVec, topK), nil
}

func (u *UnifiedDB) VectorDeleteMessage(ctx context.Context, collectionName string, id string) error {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return err
	}

	c.mu.Lock()
	for i, doc := range c.Documents {
		if doc.ID == id {
			c.Documents = append(c.Documents[:i], c.Documents[i+1:]...)
			c.mu.Unlock()
			c.saveDocumentsToFile()
			return nil
		}
	}
	c.mu.Unlock()
	return nil
}

func (u *UnifiedDB) VectorGetCollectionCount(collectionName string) int {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

func (u *UnifiedDB) VectorGetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return []DocumentEntry{}, 0
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.Documents)
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
	for i := offset; i < end; i++ {
		entries[i-offset] = DocumentEntry{
			ID:      c.Documents[i].ID,
			Role:    c.Documents[i].Role,
			Content: c.Documents[i].Content,
		}
	}
	return entries, total
}

func (u *UnifiedDB) VectorGetEntryCount(collectionName string) int {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Documents)
}

// VectorHasSyncMismatch 检测集合内是否有文档向量缺失或维度与集合锁定维度不符
func (u *UnifiedDB) VectorHasSyncMismatch(collectionName string) bool {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, doc := range c.Documents {
		if len(doc.Embedding) != c.Dimension {
			return true
		}
	}
	return false
}

// VectorRebuildEntries 删除向量缺失或维度不符的文档，重新持久化
// ctx 保留以兼容签名，当前实现不调用嵌入服务
func (u *UnifiedDB) VectorRebuildEntries(ctx context.Context, collectionName string) (int, error) {
	_ = ctx
	c, err := u.getCollection(collectionName)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	original := len(c.Documents)
	filtered := make([]VectorDocument, 0, original)
	removed := 0
	for _, doc := range c.Documents {
		if len(doc.Embedding) != c.Dimension {
			removed++
			continue
		}
		filtered = append(filtered, doc)
	}
	c.Documents = filtered
	c.mu.Unlock()

	if removed > 0 {
		c.saveDocumentsToFile()
		logger.Info("Storage", "集合 [%s] 重建完成, 原始 %d 条, 删除 %d 条维度不符, 剩余 %d 条",
			collectionName, original, removed, len(filtered))
	} else {
		logger.Info("Storage", "集合 [%s] 重建完成, 无异常文档, 共 %d 条", collectionName, original)
	}

	return len(filtered), nil
}

// VectorListCollections 返回所有已加载集合的名称
func (u *UnifiedDB) VectorListCollections() []string {
	u.collectionsMu.RLock()
	defer u.collectionsMu.RUnlock()
	names := make([]string, 0, len(u.collections))
	for name := range u.collections {
		names = append(names, name)
	}
	return names
}

// VectorGetCollectionInfo 返回集合元信息（模型、维度、文档数）
func (u *UnifiedDB) VectorGetCollectionInfo(collectionName string) (model string, dimension int, count int, err error) {
	c, err := u.getCollection(collectionName)
	if err != nil {
		return "", 0, 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Model, c.Dimension, len(c.Documents), nil
}

// =============================================================================
// Collection 持久化方法
// =============================================================================

// loadDocumentsFromFile 从 documents.json 加载文档到集合内存
func (c *Collection) loadDocumentsFromFile() {
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("Storage", "集合 [%s] 读取 documents.json 失败: %v", c.Name, err)
		}
		return
	}

	if len(data) == 0 {
		return
	}

	var docs []VectorDocument
	if err := json.Unmarshal(data, &docs); err != nil {
		logger.Warn("Storage", "集合 [%s] documents.json 解析失败: %v", c.Name, err)
		return
	}

	c.mu.Lock()
	c.Documents = docs
	c.mu.Unlock()

	maxNum := 0
	for _, doc := range docs {
		var num int
		if _, scanErr := fmt.Sscanf(doc.ID, "msg-%d", &num); scanErr == nil && num > maxNum {
			maxNum = num
		}
	}
	if maxNum > c.idCounter {
		c.idCounter = maxNum
	}
}

// saveDocumentsToFile 原子化持久化文档：写临时文件 + rename
// Windows 上 rename 不能覆盖已存在文件，先 Remove 再 Rename
func (c *Collection) saveDocumentsToFile() {
	c.mu.RLock()
	data, err := json.MarshalIndent(c.Documents, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		logger.Error("Storage", "集合 [%s] documents 序列化失败: %v", c.Name, err)
		return
	}

	tmpPath := c.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.Error("Storage", "集合 [%s] 临时文件写入失败: %v", c.Name, err)
		return
	}

	// Windows: Remove + Rename 模拟原子替换
	os.Remove(c.filePath)
	if err := os.Rename(tmpPath, c.filePath); err != nil {
		logger.Error("Storage", "集合 [%s] 原子重命名失败: %v", c.Name, err)
	}
}

// =============================================================================
// 全局包装函数 — 多集合架构
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

// VectorInitInstance 全局包装 — 初始化向量实例（不创建任何集合）
func VectorInitInstance(baseURL string, apiKey string) error {
	if Unified == nil {
		if err := InitUnifiedDB(*config.SQLDBPath, *config.VectorDBDir); err != nil {
			return err
		}
	}
	return Unified.VectorInitInstance(baseURL, apiKey)
}

// CollectionInit 全局包装 — 创建或打开指定名称的集合（探针定维度）
func CollectionInit(ctx context.Context, name string, modelName string) error {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.CollectionInit(ctx, name, modelName)
}

func AddMessage(ctx context.Context, collectionName string, role string, content string) error {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorAddMessageSilent(ctx, collectionName, role, content)
}

func AddMessageWithID(ctx context.Context, collectionName string, role string, content string) (string, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return "", fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorAddMessage(ctx, collectionName, role, content)
}

func QueryMessagesWithContent(ctx context.Context, collectionName string, queryText string, topK int) ([]VectorQueryResult, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return nil, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorQueryMessagesWithContent(ctx, collectionName, queryText, topK)
}

func DeleteMessage(ctx context.Context, collectionName string, id string) error {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorDeleteMessage(ctx, collectionName, id)
}

func GetCollectionCount(collectionName string) int {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0
	}
	return Unified.VectorGetCollectionCount(collectionName)
}

func GetDocuments(collectionName string, offset int, limit int) ([]DocumentEntry, int) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return []DocumentEntry{}, 0
	}
	return Unified.VectorGetDocuments(collectionName, offset, limit)
}

func GetEntryCount(collectionName string) int {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0
	}
	return Unified.VectorGetEntryCount(collectionName)
}

func HasSyncMismatch(collectionName string) bool {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return false
	}
	return Unified.VectorHasSyncMismatch(collectionName)
}

func RebuildEntries(ctx context.Context, collectionName string) (int, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return 0, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorRebuildEntries(ctx, collectionName)
}

// VectorListCollections 全局包装 — 列出所有已加载集合名
func VectorListCollections() []string {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return []string{}
	}
	return Unified.VectorListCollections()
}

// VectorGetCollectionInfo 全局包装 — 获取集合的模型、维度、文档数
func VectorGetCollectionInfo(collectionName string) (string, int, int, error) {
	if Unified == nil || !Unified.IsVectorInitialized() {
		return "", 0, 0, fmt.Errorf("向量数据库未初始化, 请先调用 VectorInitInstance")
	}
	return Unified.VectorGetCollectionInfo(collectionName)
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
