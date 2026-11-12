package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RecoverOnStart 在更新器启动时恢复上次中断的事务并清理残留。
// 返回 recovered=true 表示本次执行了回滚。
func RecoverOnStart(target string) (bool, error) {
	j, err := loadJournal(target)
	if err != nil {
		return false, err
	}
	if j == nil {
		cleanupStaleWork(target)
		return false, nil
	}

	switch j.Phase {
	case phaseCommitted:
		if withinRoot(stagingRoot(target), j.Staging) {
			_ = os.RemoveAll(j.Staging)
		}
		pruneUpdateBackups(target, defaultBackupKeep)
		_ = os.Remove(journalPath(target))
		cleanupStaleWork(target)
		logf("上次安装已提交，已补做清理")
		return false, nil
	case phaseRolledBack:
		_ = os.RemoveAll(filepath.Join(trashRoot(target), j.Nonce))
		_ = os.Remove(journalPath(target))
		cleanupStaleWork(target)
		return true, nil
	default:
		logf("检测到未完成事务（%s），开始回滚到 %s", j.Phase, j.FromVersion)
		if err := RollbackJournal(j, nil); err != nil {
			return true, fmt.Errorf("启动恢复回滚失败: %w", err)
		}
		cleanupStaleWork(target)
		return true, nil
	}
}

// cleanupStaleWork 清理无事务关联的过期解压/下载目录与所有 trash 残留。
func cleanupStaleWork(target string) {
	cutoff := time.Now().Add(-staleWorkAge)
	for _, root := range []string{stagingRoot(target), downloadRoot(target)} {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || info.ModTime().After(cutoff) {
				continue
			}
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	entries, err := os.ReadDir(trashRoot(target))
	if err != nil {
		return
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(trashRoot(target), e.Name()))
	}
}
