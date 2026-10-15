package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSamePath 路径等价判定：更新器必须认出「自己」。
// 判错的后果是把自身镜像当成待解锁文件，白等满超时（本次故障的根因）。
func TestSamePath(t *testing.T) {
	self := selfPath()
	if self == "" {
		t.Skip("无法取得自身可执行文件路径")
	}
	if !samePath(self, self) {
		t.Fatalf("同一路径应判为相同: %s", self)
	}
	// 大小写差异不应误判（Windows 路径不区分大小写）
	if !samePath(self, upperDrive(self)) {
		t.Fatalf("大小写差异应判为同一文件: %s / %s", self, upperDrive(self))
	}
	if samePath(self, filepath.Join(filepath.Dir(self), "不存在的文件.exe")) {
		t.Fatalf("不存在或不同的文件不应判为相同")
	}
	if samePath("", self) || samePath(self, "") {
		t.Fatalf("空路径不应判为相同")
	}
}

// upperDrive 把盘符转为大写（仅当存在盘符时）
func upperDrive(p string) string {
	if len(p) >= 2 && p[1] == ':' {
		return strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

// TestConsoleHandles 控制台句柄获取的契约：取到即可写、可取；取不到返回 nil 由调用方回退。
//
// 为什么关键：重启主程序时若把标准流接到 NUL，重启后的实例会「进程在跑、前端窗口也在、
// 控制台却永远空白」，只能去任务管理器结束（实测故障）。
func TestConsoleHandles(t *testing.T) {
	in, out := consoleHandles()
	if out != nil {
		if _, err := out.WriteString(""); err != nil {
			t.Fatalf("取到的控制台输出句柄不可写: %v", err)
		}
		_ = out.Close()
	}
	if in != nil {
		_ = in.Close()
	}
	// 无论有无控制台，都不应 panic，且两者相互独立（一个取不到不影响另一个）
}

// TestRestartProgramMissingExe 重启目标不存在时应返回错误而非静默成功
func TestRestartProgramMissingExe(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "不存在的程序.exe")
	if err := restartProgram(missing, nil); err == nil {
		t.Fatalf("重启不存在的可执行文件应返回错误")
	}
}

// TestAcquireApplyLock 单实例锁：第二次获取失败；锁释放后可再次获取；
// 过期锁（进程被强杀残留）可被接管，不会把用户永久挡在更新之外。
func TestAcquireApplyLock(t *testing.T) {
	dir := t.TempDir()

	release, ok := acquireApplyLock(dir)
	if !ok {
		t.Fatalf("首次获取锁应成功")
	}
	if _, ok := acquireApplyLock(dir); ok {
		t.Fatalf("锁被持有时不应再次获取成功（并发更新会互相覆盖并各自重启程序）")
	}

	release()
	if _, ok := acquireApplyLock(dir); !ok {
		t.Fatalf("锁释放后应可再次获取")
	}

	// 过期锁接管
	stale := filepath.Join(dir, applyLockFileName)
	old := time.Now().Add(-2 * applyLockStaleAfter)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("构造过期锁失败: %v", err)
	}
	if _, ok := acquireApplyLock(dir); !ok {
		t.Fatalf("过期锁应可被接管（否则崩溃残留会永久阻断更新）")
	}
}
