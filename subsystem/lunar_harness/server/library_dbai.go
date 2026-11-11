package server

// library_dbai.go 资源库自然语言转 SQL 的编排。
//
// 编排顺序：只读打开库 → 读取表结构 → 组装提示词上下文 → 调用语句生成智能体
// → 只读分类校验 → 输出响应。纯能力（只读打开 / 读 schema / 语句分类 / 路径规整）
// 由 engine/library 提供；engine 层不再感知提示词与智能体。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/engine/library"
)

// maxDBSchemaPromptTables 自然语言转 SQL 时随提示词携带的最大表数
const maxDBSchemaPromptTables = 80

// DBAISQLHandler POST /library/api/db/ai-sql — 自然语言需求 → 一条只读 SQL。
func DBAISQLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只支持 POST", http.StatusMethodNotAllowed)
		return
	}
	var req DBSQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Error: "请求解析失败: " + err.Error()})
		return
	}
	rel := strings.TrimSpace(req.Path)
	question := strings.TrimSpace(req.Question)
	path := library.NormalizeRel(rel)
	if question == "" {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: path, Error: "请先描述想查询的内容"})
		return
	}
	if DBSQLGenerateFunc == nil {
		writeDBSQLJSON(w, http.StatusInternalServerError, DBSQLResponse{OK: false, Path: path, Error: "语句生成能力未装配，无法生成 SQL"})
		return
	}
	conn, err := library.OpenDBReadOnly(rel)
	if err != nil {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: path, Error: err.Error()})
		return
	}
	schemas, err := library.ReadDBTableSchemas(conn)
	_ = conn.Close()
	if err != nil || len(schemas) == 0 {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: path, Error: "读取表结构失败，无法生成 SQL"})
		return
	}
	sqlText, err := DBSQLGenerateFunc(r.Context(), dbSchemaPrompt(schemas), question)
	if err != nil {
		writeDBSQLJSON(w, http.StatusBadGateway, DBSQLResponse{OK: false, Path: path, Error: "生成 SQL 失败: " + err.Error()})
		return
	}
	// 模型明确表示无法生成时原样转达；其余结果必须通过只读分类才交给前端。
	if strings.HasPrefix(sqlText, "无法生成") {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{OK: false, Path: path, Error: sqlText})
		return
	}
	if _, err := library.ClassifyDBStatement(sqlText); err != nil {
		writeDBSQLJSON(w, http.StatusBadRequest, DBSQLResponse{
			OK:    false,
			Path:  path,
			SQL:   sqlText,
			Error: "生成的语句未通过只读校验：" + err.Error(),
		})
		return
	}
	writeDBSQLJSON(w, http.StatusOK, DBSQLResponse{OK: true, Path: path, SQL: sqlText})
}

// dbSchemaPrompt 汇总当前库的全部表与列，作为语句生成的上下文。
func dbSchemaPrompt(schemas []library.DBTableSchema) string {
	var out strings.Builder
	used := 0
	for _, schema := range schemas {
		if used >= maxDBSchemaPromptTables {
			out.WriteString("（其余表已省略）\n")
			break
		}
		kind := "表"
		if schema.Table.Type == "view" {
			kind = "视图"
		}
		fmt.Fprintf(&out, "%s %s：\n", kind, schema.Table.Name)
		for _, column := range schema.Columns {
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

// writeDBSQLJSON 输出自然语言转 SQL 的 JSON 响应（保持迁移前的响应头）。
func writeDBSQLJSON(w http.ResponseWriter, status int, body DBSQLResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
