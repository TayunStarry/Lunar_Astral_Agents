package library

// dbquery.go 数据库只读查询：语句分类、多语句拒绝与结果表格化。
//
// 当前版本永久只读：只放行 SELECT / PRAGMA 查询 / EXPLAIN，写操作一律在后端拦截；
// 连接仍走 openDBReadOnly（mode=ro + _query_only=1），不新增任何可写连接。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// dbWriteKeywords 任一出现在语句中即判定为写操作（字符串与标识符已在分类前剥离）。
var dbWriteKeywords = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true, "DROP": true, "ALTER": true,
	"CREATE": true, "VACUUM": true, "ATTACH": true, "DETACH": true, "REPLACE": true,
	"REINDEX": true, "TRIGGER": true, "ANALYZE": true, "BEGIN": true, "COMMIT": true,
	"ROLLBACK": true, "SAVEPOINT": true, "RELEASE": true,
}

// dbReadPragmas 只读 PRAGMA 白名单（仅查询形态，赋值一律拒绝）。
var dbReadPragmas = map[string]bool{
	"table_info": true, "table_xinfo": true, "table_list": true,
	"index_list": true, "index_info": true, "index_xinfo": true,
	"foreign_key_list": true, "foreign_key_check": true, "database_list": true,
	"collation_list": true, "function_list": true, "module_list": true,
	"pragma_list": true, "compile_options": true, "encoding": true,
	"page_count": true, "page_size": true, "freelist_count": true,
	"schema_version": true, "user_version": true, "journal_mode": true,
	"integrity_check": true, "quick_check": true, "query_only": true,
}

// DBQueryHandler POST /library/api/db/query — 执行一条只读 SQL 并返回表格数据。
func DBQueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	var req DBQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, DBQueryResponse{OK: false, Error: "请求解析失败: " + err.Error()})
		return
	}
	rel := strings.TrimSpace(req.Path)
	statement := strings.TrimSpace(req.SQL)
	if statement == "" {
		writeJSON(w, http.StatusBadRequest, DBQueryResponse{OK: false, Path: normalizeRel(rel), Error: "SQL 不能为空"})
		return
	}
	// 先在后端做语句分类：写操作与多语句在这里被拒绝，不依赖前端校验。
	if _, err := classifyDBStatement(statement); err != nil {
		writeJSON(w, http.StatusBadRequest, DBQueryResponse{OK: false, Path: normalizeRel(rel), SQL: statement, Error: err.Error()})
		return
	}
	start := time.Now()
	conn, err := openDBReadOnly(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBQueryResponse{OK: false, Path: normalizeRel(rel), SQL: statement, Error: err.Error()})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(r.Context(), dbQueryTimeout)
	defer cancel()
	columns, rows, truncated, err := runDBQuery(ctx, conn, statement)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBQueryResponse{OK: false, Path: normalizeRel(rel), SQL: statement, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, DBQueryResponse{
		OK:        true,
		Path:      normalizeRel(rel),
		SQL:       statement,
		Columns:   columns,
		Rows:      rows,
		RowCount:  len(rows),
		Truncated: truncated,
		ElapsedMS: time.Since(start).Milliseconds(),
	})
}

// runDBQuery 执行只读语句并读取结果，超过行数上限时截断。
func runDBQuery(ctx context.Context, conn *sql.DB, statement string) ([]string, [][]any, bool, error) {
	rows, err := conn.QueryContext(ctx, statement)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, false, fmt.Errorf("查询超时（%s），请缩小查询范围", dbQueryTimeout)
		}
		return nil, nil, false, fmt.Errorf("查询失败: %w", err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, false, fmt.Errorf("读取结果列失败: %w", err)
	}
	result := make([][]any, 0, 32)
	truncated := false
	for rows.Next() {
		if len(result) >= maxDBQueryRows {
			truncated = true
			break
		}
		holders := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range holders {
			pointers[i] = &holders[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, nil, false, fmt.Errorf("读取结果失败: %w", err)
		}
		row := make([]any, len(columns))
		for i, value := range holders {
			row[i] = normalizeDBValue(value)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, nil, false, fmt.Errorf("查询超时（%s），请缩小查询范围", dbQueryTimeout)
		}
		return nil, nil, false, fmt.Errorf("读取结果失败: %w", err)
	}
	return columns, result, truncated, nil
}

// classifyDBStatement 只读分类：仅放行单条 SELECT / PRAGMA 查询 / EXPLAIN（含 WITH … SELECT）。
func classifyDBStatement(text string) (string, error) {
	code, count := stripDBSQLNoise(text)
	if count == 0 {
		return "", fmt.Errorf("SQL 不能为空")
	}
	if count > 1 {
		return "", fmt.Errorf("一次只能执行一条语句（检测到多条以分号分隔的语句）")
	}
	words := dbWords(code)
	if len(words) == 0 {
		return "", fmt.Errorf("SQL 不能为空")
	}
	switch words[0] {
	case "SELECT", "EXPLAIN":
		if word := findDBWriteWord(words[1:]); word != "" {
			return "", writeNotAllowed(word)
		}
		return words[0], nil
	case "PRAGMA":
		if strings.Contains(code, "=") {
			return "", fmt.Errorf("当前版本只允许 PRAGMA 查询，不允许赋值")
		}
		// 兼容 PRAGMA main.table_info(...) 的库名前缀写法（dbWords 已转大写，白名单按小写查）。
		allowed := len(words) > 1 && dbReadPragmas[strings.ToLower(words[1])]
		if !allowed && len(words) > 2 {
			allowed = dbReadPragmas[strings.ToLower(words[2])]
		}
		if !allowed {
			return "", fmt.Errorf("当前版本只允许 PRAGMA 查询类语句（如 table_info / index_list / database_list）")
		}
		return words[0], nil
	}
	if dbWriteKeywords[words[0]] {
		return "", writeNotAllowed(words[0])
	}
	return "", fmt.Errorf("只支持 SELECT / PRAGMA 查询 / EXPLAIN 语句")
}

// writeNotAllowed 统一写操作拒绝文案。
func writeNotAllowed(keyword string) error {
	return fmt.Errorf("当前版本不允许写操作（%s）：数据库仅支持只读查询", keyword)
}

// stripDBSQLNoise 剥离字符串、标识符与注释，返回用于分类的代码文本与语句数量。
// 单引号字符串、双引号 / 反引号 / 方括号标识符会被替换为空白，避免把字面量误判成关键字。
func stripDBSQLNoise(text string) (string, int) {
	runes := []rune(text)
	var out strings.Builder
	statements := 0
	hasContent := false
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; {
		case c == '\'' || c == '"' || c == '`':
			hasContent = true
			out.WriteByte(' ')
			for i++; i < len(runes); i++ {
				if runes[i] != c {
					continue
				}
				if i+1 < len(runes) && runes[i+1] == c {
					i++
					continue
				}
				break
			}
		case c == '[':
			hasContent = true
			out.WriteByte(' ')
			for i++; i < len(runes) && runes[i] != ']'; i++ {
			}
		case c == '-' && i+1 < len(runes) && runes[i+1] == '-':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			out.WriteByte(' ')
		case c == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for i+1 < len(runes) && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			i++
			out.WriteByte(' ')
		case c == ';':
			if hasContent {
				statements++
				hasContent = false
			}
			out.WriteByte(' ')
		default:
			if !unicode.IsSpace(c) {
				hasContent = true
			}
			out.WriteRune(c)
		}
	}
	if hasContent {
		statements++
	}
	return out.String(), statements
}

// dbWords 提取代码中的关键字候选（字母 / 数字 / 下划线），统一大写。
func dbWords(code string) []string {
	words := make([]string, 0, 16)
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			words = append(words, strings.ToUpper(current.String()))
			current.Reset()
		}
	}
	for _, c := range code {
		if c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) {
			current.WriteRune(c)
			continue
		}
		flush()
	}
	flush()
	return words
}

func findDBWriteWord(words []string) string {
	for _, word := range words {
		if dbWriteKeywords[word] {
			return word
		}
	}
	return ""
}
