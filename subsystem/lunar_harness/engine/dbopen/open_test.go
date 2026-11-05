package dbopen

// open_test.go opener 的表驱动用例：默认参数、显式 busy_timeout / journal_mode、
// 外键、连接池上限、父目录自动创建与建目录失败的错误类型。

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenDSNParams 表驱动校验 DSN 参数落到 PRAGMA。
func TestOpenDSNParams(t *testing.T) {
	cases := []struct {
		name     string
		opt      Options
		wantBusy int64
		wantMode string
		wantFK   int64
	}{
		{"默认 busy_timeout + WAL", Options{}, 10000, "wal", 0},
		{"显式 busy_timeout", Options{BusyTimeoutMS: 2500}, 2500, "wal", 0},
		{"显式 journal_mode", Options{JournalMode: "DELETE"}, 10000, "delete", 0},
		{"外键开启", Options{ForeignKeys: true}, 10000, "wal", 1},
		// 裸 DSN 沿用 go-sqlite3 驱动的默认 busy_timeout（5000ms）与默认 journal_mode
		{"裸 DSN 不带任何参数", Options{NoDSNParams: true}, 5000, "delete", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "dsn.db"), tc.opt)
			if err != nil {
				t.Fatalf("打开失败: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			var busy int64
			if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
				t.Fatalf("读取 busy_timeout 失败: %v", err)
			}
			if busy != tc.wantBusy {
				t.Fatalf("busy_timeout 应为 %d，实际 %d", tc.wantBusy, busy)
			}
			var mode string
			if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
				t.Fatalf("读取 journal_mode 失败: %v", err)
			}
			if mode != tc.wantMode {
				t.Fatalf("journal_mode 应为 %q，实际 %q", tc.wantMode, mode)
			}
			var fk int64
			if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
				t.Fatalf("读取 foreign_keys 失败: %v", err)
			}
			if fk != tc.wantFK {
				t.Fatalf("foreign_keys 应为 %d，实际 %d", tc.wantFK, fk)
			}
		})
	}
}

// TestOpenPoolLimits 显式设置单连接上限后第二次取连接会阻塞；未设置时可直接并发取用。
func TestOpenPoolLimits(t *testing.T) {
	ctx := context.Background()

	t.Run("单连接上限", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "pool.db"), Options{MaxOpenConns: 1, MaxIdleConns: 1})
		if err != nil {
			t.Fatalf("打开失败: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("取第一个连接失败: %v", err)
		}
		waitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		if _, err := db.Conn(waitCtx); err == nil {
			_ = conn.Close()
			t.Fatal("上限为 1 时第二次取连接应阻塞至超时")
		}
		_ = conn.Close()
	})

	t.Run("未设置保持默认", func(t *testing.T) {
		db, err := Open(filepath.Join(t.TempDir(), "pool.db"), Options{})
		if err != nil {
			t.Fatalf("打开失败: %v", err)
		}
		t.Cleanup(func() { _ = db.Close() })
		first, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("取第一个连接失败: %v", err)
		}
		defer first.Close()
		second, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("未设置上限时第二个连接应立即可取: %v", err)
		}
		_ = second.Close()
	})
}

// TestOpenCreateDir 自动创建父目录并可正常读写。
func TestOpenCreateDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "dir.db")
	db, err := Open(path, Options{CreateDir: true})
	if err != nil {
		t.Fatalf("打开失败: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("父目录未创建: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t (id, v) VALUES (1, 'ok')"); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	var v string
	if err := db.QueryRow("SELECT v FROM t WHERE id = 1").Scan(&v); err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if v != "ok" {
		t.Fatalf("读回值应为 ok，实际 %q", v)
	}
}

// TestOpenCreateDirError 父目录无法创建时返回 *DirError，调用方可取回底层错误。
func TestOpenCreateDirError(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("准备阻塞文件失败: %v", err)
	}
	_, err := Open(filepath.Join(blocker, "x.db"), Options{CreateDir: true})
	if err == nil {
		t.Fatal("父目录不可创建时应返回错误")
	}
	if _, ok := DirCause(err); !ok {
		t.Fatalf("错误应可识别为建目录失败: %v", err)
	}
}
