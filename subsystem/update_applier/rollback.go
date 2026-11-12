package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// RollbackJournal 按事务日志做确定性回滚：
//   - 有备份的文件：目标先移入 trash，再把备份移回；
//   - 新增文件：直接移入 trash；
//   - 尚未备份的文件保持原状。
//
// 回滚是幂等的，可在任意中断点重复执行。
func RollbackJournal(j *Journal, report func(Progress)) error {
	if report == nil {
		report = func(Progress) {}
	}
	if j == nil {
		return fmt.Errorf("缺少事务日志")
	}
	if j.Phase == phaseCommitted {
		return fmt.Errorf("事务已提交，不能回滚")
	}
	_ = setJournalPhase(j.Target, j, phaseRollingBack)

	trash := filepath.Join(trashRoot(j.Target), j.Nonce)
	total := len(j.Files)
	for i, f := range j.Files {
		if err := rollbackOne(j, f, trash); err != nil {
			return fmt.Errorf("回滚 %s 失败: %w", f.Path, err)
		}
		report(Progress{Phase: phaseRollingBack, Done: i + 1, Total: total, Percent: percent(i+1, total), Message: f.Path})
	}

	for i := len(j.CreatedDirs) - 1; i >= 0; i-- {
		_ = os.Remove(j.CreatedDirs[i])
	}

	if j.StateBefore != nil {
		if err := writeState(j.Target, j.StateBefore); err != nil {
			return fmt.Errorf("恢复安装状态失败: %w", err)
		}
	} else {
		if err := os.Remove(statePath(j.Target)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("清除安装状态失败: %w", err)
		}
	}

	if withinRoot(stagingRoot(j.Target), j.Staging) {
		_ = os.RemoveAll(j.Staging)
	}
	_ = os.RemoveAll(trash)
	_ = os.RemoveAll(filepath.Join(downloadRoot(j.Target), j.Nonce))
	if withinRoot(backupRoot(j.Target), j.Backup) {
		_ = os.RemoveAll(j.Backup)
	}
	_ = setJournalPhase(j.Target, j, phaseRolledBack)
	_ = os.Remove(journalPath(j.Target))
	report(Progress{Phase: phaseRolledBack, Done: total, Total: total, Percent: 100, Message: "已回滚到安装前状态"})
	return nil
}

// rollbackOne 回滚单个文件。
func rollbackOne(j *Journal, f FileEntry, trash string) error {
	dest, err := safeJoin(j.Target, f.Path)
	if err != nil {
		return err
	}
	backup, err := safeJoin(j.Backup, f.Path)
	if err != nil {
		return err
	}
	if f.HadOld {
		if !fileExists(backup) {
			// 尚未备份：目标仍是旧文件（或已被外部删除），无需处理。
			return nil
		}
		if fileExists(dest) {
			if err := moveToTrash(trash, dest, f.Path); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return moveWithRetry(backup, dest)
	}
	if fileExists(dest) {
		return moveToTrash(trash, dest, f.Path)
	}
	return nil
}

// moveToTrash 把文件移入事务 trash 目录（同卷 rename）。
func moveToTrash(trash, src, rel string) error {
	dst, err := safeJoin(trash, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return moveWithRetry(src, dst)
}
