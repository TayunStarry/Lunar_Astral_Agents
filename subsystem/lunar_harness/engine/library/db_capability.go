package library

// db_capability.go 资源库数据库能力的对外出口。
//
// 自然语言转 SQL 的编排已上移到 server 层；engine/library 只暴露纯能力：
// 只读打开、表结构读取、只读语句分类与路径规整。本文件不拼提示词、不调用智能体。

import "database/sql"

// OpenDBReadOnly 以只读方式打开 database/ 下的库。
func OpenDBReadOnly(rel string) (*sql.DB, error) {
	return openDBReadOnly(rel)
}

// ReadDBTableSchemas 读取库中全部表与视图的列结构；列结构读取失败的表跳过。
func ReadDBTableSchemas(conn *sql.DB) ([]DBTableSchema, error) {
	tables, err := readDBTables(conn)
	if err != nil {
		return nil, err
	}
	schemas := make([]DBTableSchema, 0, len(tables))
	for _, table := range tables {
		columns, err := readDBColumns(conn, table.Name)
		if err != nil {
			continue
		}
		schemas = append(schemas, DBTableSchema{Table: table, Columns: columns})
	}
	return schemas, nil
}

// ClassifyDBStatement 只读分类：仅放行单条 SELECT / PRAGMA 查询 / EXPLAIN。
func ClassifyDBStatement(text string) (string, error) {
	return classifyDBStatement(text)
}

// NormalizeRel 规整相对 LocalDir 的路径（统一斜杠、去掉前缀与多余层级）。
func NormalizeRel(rel string) string {
	return normalizeRel(rel)
}
