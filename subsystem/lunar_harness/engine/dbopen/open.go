package dbopen

// open.go 全仓唯一的 SQLite 打开实现：DSN 参数、连接池与父目录创建都只在这里处理。

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"

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
