package StarLTP

// ==== SQLite 数据库 database.*（allow-database） ====
// 命名空间：默认共用 local_data/database/knowledge.db；database.namespace(name)
// 打开独立 <name>.db 实现真隔离。事务（transaction）与迁移（migrate）均以库为单位，
// 任意库（共享/命名空间）都可用。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"LunarSubsystem/GeneralConfig"

	"github.com/dop251/goja"
)

// ==== 句柄管理（按库名缓存，惰性打开） ====

var (
	// dbMu 保护 dbHandles 的并发打开。
	dbMu sync.Mutex
	// dbHandles 库名（空串 = 共享 knowledge.db）→ 连接句柄。
	dbHandles map[string]*sql.DB
)

// nsValidName 命名空间名白名单（防路径穿越，仅允许字母数字-_）。
var nsValidName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// databaseDir LTP9 数据库目录（可执行目录/local_data/database）。
func databaseDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return filepath.Join("local_data", "database")
	}
	return filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "database")
}

// ensureDB 打开默认共享库 knowledge.db。
func ensureDB() (*sql.DB, error) {
	return nsDB("")
}

// nsDB 打开（或复用）指定命名空间的 SQLite 库；name 为空 → knowledge.db。
// 每库自动开启 WAL，并确保迁移记录表 _migrations 就绪。
func nsDB(name string) (*sql.DB, error) {
	file := "knowledge.db"
	if name != "" {
		if !nsValidName.MatchString(name) {
			return nil, fmt.Errorf("非法命名空间名: %s（仅允许字母数字-_）", name)
		}
		file = name + ".db"
	}
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbHandles == nil {
		dbHandles = map[string]*sql.DB{}
	}
	if db, ok := dbHandles[name]; ok {
		return db, nil
	}
	dir := databaseDir()
	_ = os.MkdirAll(dir, 0755)
	db, err := sql.Open("sqlite3", filepath.Join(dir, file))
	if err != nil {
		return nil, err
	}
	if _, err = db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, err
	}
	// 迁移记录表：migrate(name, upSql) 按名称记录已应用版本
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS _migrations (name TEXT PRIMARY KEY, applied_at INTEGER)"); err != nil {
		db.Close()
		return nil, err
	}
	dbHandles[name] = db
	return db, nil
}

// ==== 查询/执行（*sql.DB 与 *sql.Tx 通用，供事务复用） ====

// sqlQueryer 查询接口（*sql.DB / *sql.Tx 均满足）。
type sqlQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

// sqlExecer 执行接口（*sql.DB / *sql.Tx 均满足）。
type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// databaseQuery 在共享库上执行 SELECT（Result 统一风格）。
func databaseQuery(q string, params []any) (any, error) {
	db, err := ensureDB()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return dbQueryOn(db, q, params)
}

// databaseExec 在共享库上执行写语句（Result 统一风格）。
func databaseExec(q string, params []any) (any, error) {
	db, err := ensureDB()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return dbExecOn(db, q, params)
}

// dbQueryOn 在指定库/事务上执行 SELECT，返回 { success, rows }；每行为「列名 → 值」对象。
func dbQueryOn(q sqlQueryer, query string, params []any) (any, error) {
	rows, err := q.Query(query, anySlice(params)...)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		row := map[string]any{}
		for i, c := range cols {
			row[c] = normalizeVal(vals[i])
		}
		out = append(out, row)
	}
	return map[string]any{"success": true, "rows": out}, nil
}

// dbExecOn 在指定库/事务上执行写语句，返回 { success, rows_affected }。
func dbExecOn(e sqlExecer, query string, params []any) (any, error) {
	res, err := e.Exec(query, anySlice(params)...)
	if err != nil {
		return map[string]any{"success": false, "rows_affected": 0, "error": err.Error()}, nil
	}
	rows, _ := res.RowsAffected()
	return map[string]any{"success": true, "rows_affected": rows}, nil
}

func anySlice(in []any) []any {
	if in == nil {
		return []any{}
	}
	return in
}

// normalizeVal 把 sqlite 返回的类型规整为 JS 友好值（[]byte → string）。
func normalizeVal(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case int64:
		return float64(t)
	}
	return v
}

// ==== 事务（database.transaction，同步回调） ====

// databaseTransaction 共享库上的同步事务。fn 接收 { query, exec }（绑定到事务）：
// 正常返回 → 提交并回传 { success, result: fn返回值 }；
// fn 抛 JS 异常 / 返回 Promise（事务内禁异步）/ Go panic → 回滚并返回 { success:false, error }。
// 同步阻塞模型：本函数在插件事件循环线程内执行，回调期间不调度其它 JS。
func databaseTransaction(vm *goja.Runtime, fn goja.Value) (any, error) {
	db, err := ensureDB()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return databaseTransactionOn(vm, db, fn)
}

// databaseTransactionOn 指定库上的同步事务（databaseTransaction 的库参数化版本）。
func databaseTransactionOn(vm *goja.Runtime, db *sql.DB, fn goja.Value) (any, error) {
	f, ok := goja.AssertFunction(fn)
	if !ok {
		return rwResult{Success: false, Error: "transaction 需传入函数"}, nil
	}
	if db == nil {
		var derr error
		if db, derr = ensureDB(); derr != nil {
			return rwResult{Success: false, Error: derr.Error()}, nil
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	txObj := vm.NewObject()
	txObj.Set("query", func(q string, params []any) (any, error) { return dbQueryOn(tx, q, params) })
	txObj.Set("exec", func(q string, params []any) (any, error) { return dbExecOn(tx, q, params) })

	// JS throw 经 AssertFunction 转为返回值 error；recover 兜底 goja 内部 panic
	res, ferr := func() (res goja.Value, ferr error) {
		defer func() {
			if r := recover(); r != nil {
				ferr = fmt.Errorf("事务回调异常: %v", r)
			}
		}()
		return f(goja.Undefined(), txObj)
	}()
	if ferr != nil {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: ferr.Error()}, nil
	}
	// 事务回调内禁止异步：Promise 无法在提交前结算，回滚防止悬挂事务
	if _, isPromise := res.Export().(*goja.Promise); isPromise {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: "事务回调不支持异步（Promise）"}, nil
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "result": res.Export()}, nil
}

// ==== 迁移（database.migrate，按名称记录） ====

// dbMigrateOn 按名称执行一次迁移：_migrations 表未记录则在事务内执行 upSql
// （go-sqlite3 支持单字符串多语句；脚本内禁含 BEGIN/COMMIT）并插入记录。
// 返回 { success, applied }（applied=false 表示此前已应用，本次跳过）。
func dbMigrateOn(db *sql.DB, name, upSQL string) (any, error) {
	if strings.TrimSpace(name) == "" {
		return rwResult{Success: false, Error: "迁移名必填"}, nil
	}
	if strings.TrimSpace(upSQL) == "" {
		return rwResult{Success: false, Error: "迁移 SQL 必填"}, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM _migrations WHERE name = ?", name).Scan(&n); err != nil {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if n > 0 {
		_ = tx.Rollback()
		return map[string]any{"success": true, "applied": false}, nil
	}
	if _, err := tx.Exec(upSQL); err != nil {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: "迁移执行失败: " + err.Error()}, nil
	}
	if _, err := tx.Exec("INSERT INTO _migrations (name, applied_at) VALUES (?, ?)", name, time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if err := tx.Commit(); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "applied": true}, nil
}

// ==== 命名空间作用域（database.namespace） ====

// databaseNamespace 构造命名空间作用域对象 { query, exec, transaction, migrate }，
// 全部操作落在独立 <name>.db 上。名称非法时返回 error（由 goja 转为 JS 异常）。
func databaseNamespace(vm *goja.Runtime, name string) (*goja.Object, error) {
	db, err := nsDB(name)
	if err != nil {
		return nil, err
	}
	n := vm.NewObject()
	n.Set("query", func(q string, params []any) (any, error) { return dbQueryOn(db, q, params) })
	n.Set("exec", func(q string, params []any) (any, error) { return dbExecOn(db, q, params) })
	n.Set("transaction", func(fn goja.Value) (any, error) { return databaseTransactionOn(vm, db, fn) })
	n.Set("migrate", func(mName, upSQL string) (any, error) { return dbMigrateOn(db, mName, upSQL) })
	return n, nil
}
