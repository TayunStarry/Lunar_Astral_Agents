package library

// dbai.go 自然语言转 SQL：把当前打开库的表结构与列信息作为上下文交给语句生成智能体，
// 只返回一条只读 SQL（或明确的「无法生成」说明），不执行。

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/agents/SQLWriter"
)

// DBAISQLHandler POST /library/api/db/ai-sql — 自然语言需求 → 一条只读 SQL。
func DBAISQLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	var req DBSQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Error: "请求解析失败: " + err.Error()})
		return
	}
	rel := strings.TrimSpace(req.Path)
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: normalizeRel(rel), Error: "请先描述想查询的内容"})
		return
	}
	conn, err := openDBReadOnly(rel)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: normalizeRel(rel), Error: err.Error()})
		return
	}
	schema := dbSchemaPrompt(conn)
	_ = conn.Close()
	if schema == "" {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: normalizeRel(rel), Error: "读取表结构失败，无法生成 SQL"})
		return
	}
	sqlText, err := SQLWriter.Generate(r.Context(), schema, question)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, DBSQLResponse{OK: false, Path: normalizeRel(rel), Error: "生成 SQL 失败: " + err.Error()})
		return
	}
	// 模型明确表示无法生成时原样转达；其余结果必须通过只读分类才交给前端。
	if strings.HasPrefix(sqlText, "无法生成") {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: normalizeRel(rel), Error: sqlText})
		return
	}
	if _, err := classifyDBStatement(sqlText); err != nil {
		writeJSON(w, http.StatusBadRequest, DBSQLResponse{
			OK:    false,
			Path:  normalizeRel(rel),
			SQL:   sqlText,
			Error: "生成的语句未通过只读校验：" + err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, DBSQLResponse{OK: true, Path: normalizeRel(rel), SQL: sqlText})
}

// dbSchemaPrompt 汇总当前库的全部表与列，作为语句生成的上下文。
func dbSchemaPrompt(conn *sql.DB) string {
	tables, err := readDBTables(conn)
	if err != nil || len(tables) == 0 {
		return ""
	}
	var out strings.Builder
	used := 0
	for _, table := range tables {
		if used >= maxDBSchemaPromptTables {
			out.WriteString("（其余表已省略）\n")
			break
		}
		columns, err := readDBColumns(conn, table.Name)
		if err != nil {
			continue
		}
		kind := "表"
		if table.Type == "view" {
			kind = "视图"
		}
		fmt.Fprintf(&out, "%s %s：\n", kind, table.Name)
		for _, column := range columns {
			typ := column.Type
			if typ == "" {
				typ = "未声明类型"
			}
			flags := ""
			if column.PK {
				flags += " 主键"
			}
			if column.NotNull {
				flags += " 非空"
			}
			fmt.Fprintf(&out, "  - %s %s%s\n", column.Name, typ, flags)
		}
		used++
	}
	return strings.TrimSpace(out.String())
}
