package library

// dbapi.go 资源库数据库只读预览：表清单 / 表结构 / 分页行数据。
//
// 打开一律走 dbopen 的只读选项（mode=ro + _query_only=1），不建表、不迁移、
// 不执行 VACUUM 或任何写语句；表名与排序列都先做白名单校验再拼接标识符。

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/dbopen"
)

// DBTablesHandler GET /library/api/db/tables?path=database/library.db — 表与视图清单。
func DBTablesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	conn, err := openDBReadOnly(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBTablesResponse{OK: false, Path: normalizeRel(rel), Error: err.Error()})
		return
	}
	defer conn.Close()
	tables, err := readDBTables(conn)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DBTablesResponse{OK: false, Path: normalizeRel(rel), Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, DBTablesResponse{OK: true, Path: normalizeRel(rel), Tables: tables})
}

// DBSchemaHandler GET /library/api/db/schema?path=...&table=... — 单表列结构。
func DBSchemaHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	table := strings.TrimSpace(r.URL.Query().Get("table"))
	if table == "" {
		writeJSON(w, http.StatusBadRequest, DBSchemaResponse{OK: false, Path: normalizeRel(rel), Error: "缺少 table"})
		return
	}
	conn, err := openDBReadOnly(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBSchemaResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	defer conn.Close()
	if _, err := readDBTableKind(conn, table); err != nil {
		writeJSON(w, http.StatusBadRequest, DBSchemaResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	columns, err := readDBColumns(conn, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DBSchemaResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, DBSchemaResponse{OK: true, Path: normalizeRel(rel), Table: table, Columns: columns})
}

// DBRowsHandler GET /library/api/db/rows?path=...&table=...&offset=0&limit=100&sort=&order=asc
func DBRowsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只支持 GET", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	rel := strings.TrimSpace(query.Get("path"))
	table := strings.TrimSpace(query.Get("table"))
	if table == "" {
		writeJSON(w, http.StatusBadRequest, DBRowsResponse{OK: false, Path: normalizeRel(rel), Error: "缺少 table"})
		return
	}
	offset := queryInt(r, "offset")
	if offset < 0 {
		offset = 0
	}
	limit := queryInt(r, "limit")
	if limit <= 0 {
		limit = defaultDBPageSize
	}
	if limit > maxDBPageSize {
		limit = maxDBPageSize
	}
	order := strings.ToLower(strings.TrimSpace(query.Get("order")))
	if order != "desc" {
		order = "asc"
	}
	sortColumn := strings.TrimSpace(query.Get("sort"))

	conn, err := openDBReadOnly(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	defer conn.Close()
	if _, err := readDBTableKind(conn, table); err != nil {
		writeJSON(w, http.StatusBadRequest, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	columns, err := readDBColumns(conn, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	if sortColumn != "" && !containsString(names, sortColumn) {
		writeJSON(w, http.StatusBadRequest, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Columns: names, Error: "排序列不存在: " + sortColumn})
		return
	}
	total, err := countDBRows(conn, table)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	rows, err := readDBRows(conn, table, names, sortColumn, order, offset, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, DBRowsResponse{OK: false, Path: normalizeRel(rel), Table: table, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, DBRowsResponse{
		OK:      true,
		Path:    normalizeRel(rel),
		Table:   table,
		Columns: names,
		Rows:    rows,
		Total:   total,
		Offset:  offset,
		Limit:   limit,
		Sort:    sortColumn,
		Order:   order,
	})
}

// openDBReadOnly 以只读方式打开 database/ 下的库；连接层再补一次 query_only。
func openDBReadOnly(rel string) (*sql.DB, error) {
	full, err := resolveDBPath(rel)
	if err != nil {
		return nil, err
	}
	conn, err := dbopen.Open(full, dbopen.Options{ReadOnly: true, MaxOpenConns: 1})
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// DSN 已带 _query_only=1；这里再显式声明一次，确保单连接池上的任何语句都只读。
	if _, err := conn.Exec(`PRAGMA query_only = 1`); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("设置只读模式失败: %w", err)
	}
	return conn, nil
}

// readDBTables 返回用户表与视图（排除 sqlite_ 内部表）。
func readDBTables(conn *sql.DB) ([]DBTable, error) {
	rows, err := conn.Query(`SELECT name, type FROM sqlite_master
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("读取表清单失败: %w", err)
	}
	defer rows.Close()
	tables := make([]DBTable, 0, 16)
	for rows.Next() {
		var item DBTable
		if err := rows.Scan(&item.Name, &item.Type); err != nil {
			return nil, fmt.Errorf("读取表清单失败: %w", err)
		}
		tables = append(tables, item)
	}
	return tables, rows.Err()
}

// readDBTableKind 校验表名并返回 table / view。
func readDBTableKind(conn *sql.DB, table string) (string, error) {
	var kind string
	err := conn.QueryRow(`SELECT type FROM sqlite_master
		WHERE name = ? AND type IN ('table', 'view')`, table).Scan(&kind)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("表不存在: %s", table)
	}
	if err != nil {
		return "", fmt.Errorf("读取表信息失败: %w", err)
	}
	return kind, nil
}

// readDBColumns 通过 PRAGMA table_info 读取列名、声明类型、非空与主键标记。
func readDBColumns(conn *sql.DB, table string) ([]DBColumn, error) {
	rows, err := conn.Query(`PRAGMA table_info(` + quoteDBIdent(table) + `)`)
	if err != nil {
		return nil, fmt.Errorf("读取表结构失败: %w", err)
	}
	defer rows.Close()
	columns := make([]DBColumn, 0, 16)
	for rows.Next() {
		var (
			cid       int
			name      string
			columnTyp string
			notNull   int
			primary   int
			defaultV  sql.NullString
		)
		if err := rows.Scan(&cid, &name, &columnTyp, &notNull, &defaultV, &primary); err != nil {
			return nil, fmt.Errorf("读取表结构失败: %w", err)
		}
		columns = append(columns, DBColumn{
			Name:    name,
			Type:    columnTyp,
			NotNull: notNull != 0,
			PK:      primary != 0,
			Default: defaultV.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取表结构失败: %w", err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("表结构为空: %s", table)
	}
	return columns, nil
}

// countDBRows 统计表总行数（用于分页展示）。
func countDBRows(conn *sql.DB, table string) (int, error) {
	var total int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + quoteDBIdent(table)).Scan(&total); err != nil {
		return 0, fmt.Errorf("统计行数失败: %w", err)
	}
	return total, nil
}

// readDBRows 读取一页行数据；排序列与顺序均已在上层校验。
func readDBRows(conn *sql.DB, table string, columns []string, sortColumn, order string, offset, limit int) ([][]any, error) {
	quoted := make([]string, 0, len(columns))
	for _, name := range columns {
		quoted = append(quoted, quoteDBIdent(name))
	}
	statement := `SELECT ` + strings.Join(quoted, ", ") + ` FROM ` + quoteDBIdent(table)
	if sortColumn != "" {
		statement += ` ORDER BY ` + quoteDBIdent(sortColumn) + ` ` + strings.ToUpper(order)
	}
	statement += ` LIMIT ? OFFSET ?`

	rows, err := conn.Query(statement, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("读取行数据失败: %w", err)
	}
	defer rows.Close()
	result := make([][]any, 0, limit)
	for rows.Next() {
		holders := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range holders {
			pointers[i] = &holders[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("读取行数据失败: %w", err)
		}
		row := make([]any, len(columns))
		for i, value := range holders {
			row[i] = normalizeDBValue(value)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// normalizeDBValue 把驱动返回值转为可 JSON 化的标量：
// BLOB / 文本字节优先按 UTF-8 展示，不可读时转十六进制并截断。
func normalizeDBValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		if utf8.Valid(typed) {
			if len(typed) > maxDBCellBytes {
				return string(typed[:maxDBCellBytes]) + "…"
			}
			return string(typed)
		}
		if len(typed) > maxDBCellBytes {
			return "0x" + hex.EncodeToString(typed[:maxDBCellBytes]) + "…"
		}
		return "0x" + hex.EncodeToString(typed)
	case time.Time:
		return typed.Format("2006-01-02 15:04:05")
	default:
		return typed
	}
}

// quoteDBIdent 双引号包裹标识符，内部双引号按 SQL 规则转义。
func quoteDBIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
