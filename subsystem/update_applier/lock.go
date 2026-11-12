package main

// ==== 更新器单实例锁 ====
//
// 用户看不到反馈时的第一反应是「再点一次程序」，于是第二个更新器也会开始下载/覆盖，
// 两个进程同时改同一批文件会互相覆盖，并各自拉起主程序——结果是文件错乱 + 双实例。
//
// 锁文件放在 `.update/apply.lock`：同一目标天然互斥，锁随进程结束清除；
// 进程被强杀留下的锁按时间戳判定过期后可由下一个更新器接管。
// TUI 长时间开着时由心跳刷新时间戳，避免被误判为过期锁。

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// applyLockStaleAfter 锁过期时长：进程被强杀/OOM 后残留的锁超过该时长即可接管
	applyLockStaleAfter = 15 * time.Minute
)

// lockPath 锁文件路径
func lockPath(targetDir string) string {
	return filepath.Join(updateRoot(targetDir), applyLockFile)
}

// acquireApplyLock 获取单实例锁：返回释放函数与是否获取成功。
// 已存在且未过期的锁视为「另一个更新器正在工作」，返回 false。
func acquireApplyLock(targetDir string) (func(), bool) {
	path := lockPath(targetDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false
	}

	if fi, err := os.Stat(path); err == nil {
		if time.Since(fi.ModTime()) < applyLockStaleAfter {
			return nil, false
		}
		// 过期锁（上次被强杀留下）：接管，避免用户被残留文件永久挡住
		_ = os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, false
	}
	fmt.Fprintf(f, "pid=%d\ntime=%s\n", os.Getpid(), time.Now().Format(time.RFC3339))
	_ = f.Close()

	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(lockHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = touch(path)
			}
		}
	}()

	return func() {
		close(stop)
		_ = os.Remove(path)
	}, true
}
