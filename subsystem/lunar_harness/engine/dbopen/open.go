package dbopen

// open.go 全仓唯一的 SQLite 打开实现：DSN 参数、连接池与父目录创建都只在这里处理。

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Open 按选项打开 SQLite 库；返回的 *sql.DB 由调用方负责关闭。
// 父目录创建失败返回 *DirError，其余错误原样返回，便于调用方保留既有文案。
func Open(path string, opt Options) (*sql.DB, error) {
	if opt.CreateDir {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, &DirError{Err: err}
		}
	}
	db, err := sql.Open("sqlite3", dsn(path, opt))
	if err != nil {
		return nil, err
	}
	if opt.MaxOpenConns > 0 {
		db.SetMaxOpenConns(opt.MaxOpenConns)
	} else if opt.ReadOnly {
		// 只读预览不并发：单连接即可，也避免多个连接各自持有 WAL 读游标。
		db.SetMaxOpenConns(1)
	}
	if opt.MaxIdleConns > 0 {
		db.SetMaxIdleConns(opt.MaxIdleConns)
	}
	if opt.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(opt.ConnMaxLifetime)
	}
	return db, nil
}

// DirCause 当 err 由父目录创建失败引起时返回底层错误，否则 ok 为 false。
func DirCause(err error) (error, bool) {
	var dirErr *DirError
	if errors.As(err, &dirErr) {
		return dirErr.Err, true
	}
	return nil, false
}

// dsn 拼接 sqlite3 DSN；参数顺序与替换前的历史实现保持一致。
func dsn(path string, opt Options) string {
	if opt.NoDSNParams {
		return path
	}
	busy := opt.BusyTimeoutMS
	if busy == 0 {
		busy = DefaultBusyTimeoutMS
	}
	if opt.ReadOnly {
		// 只读：URI 形式让 mode=ro 生效；_query_only=1 在连接层拒绝任何非查询语句。
		// 不写 journal_mode，避免只读连接触发任何元数据变更。
		query := "?mode=ro&_busy_timeout=" + strconv.Itoa(busy) + "&_query_only=1"
		if opt.ForeignKeys {
			query += "&_foreign_keys=on"
		}
		return sqliteReadOnlyURI(path) + query
	}
	mode := opt.JournalMode
	if mode == "" {
		mode = DefaultJournalMode
	}
	query := "?_busy_timeout=" + strconv.Itoa(busy) + "&_journal_mode=" + mode
	if opt.ForeignKeys {
		query += "&_foreign_keys=on"
	}
	return path + query
}

// sqliteReadOnlyURI 把本地路径转成 SQLite URI 形式（file:///），
// 仅与 mode=ro 搭配使用；Windows 盘符路径同样适用。
//
// 必须先转绝对路径：config.LocalDir 是相对路径（local_data），
// 直接拼成 file:///local_data/... 会被 SQLite 解析成盘符根下的目录，
// 从而报 "unable to open database file: The system cannot find the path specified"。
func sqliteReadOnlyURI(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return "file://" + slashed
}
