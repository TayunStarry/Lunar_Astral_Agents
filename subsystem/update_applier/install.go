package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ApplyPlan 执行一次原子安装：落盘日志 → 备份旧文件 → 覆盖新文件 → 写状态 → 提交。
// 任何写入失败都会回滚到安装前状态；调用方需保证持有单实例锁。
func ApplyPlan(plan InstallPlan, report func(Progress)) (err error) {
	if report == nil {
		report = func(Progress) {}
	}
	if err := validatePlan(plan); err != nil {
		return err
	}

	j, err := buildJournal(plan)
	if err != nil {
		return fmt.Errorf("生成事务计划失败: %w", err)
	}
	if err := saveJournal(j.Target, j); err != nil {
		return fmt.Errorf("写入事务日志失败: %w", err)
	}
	report(Progress{Phase: phasePrepared, Total: len(j.Files), Message: "事务计划已落盘"})

	if err := setJournalPhase(j.Target, j, phaseBackup); err != nil {
		return fmt.Errorf("写入备份阶段失败: %w", err)
	}
	for i, f := range j.Files {
		if !f.HadOld {
			continue
		}
		if err := backupOne(j, f); err != nil {
			return failAndRollback(j, fmt.Errorf("备份 %s 失败: %w", f.Path, err), report)
		}
		report(Progress{Phase: phaseBackup, Done: i + 1, Total: len(j.Files), Percent: percent(i+1, len(j.Files)), Message: f.Path})
	}

	if err := setJournalPhase(j.Target, j, phaseApplying); err != nil {
		return failAndRollback(j, fmt.Errorf("写入覆盖阶段失败: %w", err), report)
	}
	for i, f := range j.Files {
		if err := applyOne(j, f); err != nil {
			return failAndRollback(j, fmt.Errorf("覆盖 %s 失败: %w", f.Path, err), report)
		}
		report(Progress{Phase: phaseApplying, Done: i + 1, Total: len(j.Files), Percent: percent(i+1, len(j.Files)), Message: f.Path})
	}

	if err := writeState(j.Target, j.StateAfter); err != nil {
		return failAndRollback(j, fmt.Errorf("写入安装状态失败: %w", err), report)
	}
	if err := setJournalPhase(j.Target, j, phaseCommitted); err != nil {
		return fmt.Errorf("写入提交阶段失败: %w", err)
	}
	finishCommitted(j)
	report(Progress{Phase: phaseCommitted, Done: len(j.Files), Total: len(j.Files), Percent: 100, Message: "安装完成"})
	return nil
}

// validatePlan 校验安装计划的关键字段与路径边界。
func validatePlan(plan InstallPlan) error {
	if strings.TrimSpace(plan.Target) == "" {
		return fmt.Errorf("安装计划缺少目标目录")
	}
	if strings.TrimSpace(plan.Staging) == "" {
		return fmt.Errorf("安装计划缺少解压目录")
	}
	if len(plan.Files) == 0 {
		return fmt.Errorf("安装计划没有文件")
	}
	target, err := filepath.Abs(plan.Target)
	if err != nil {
		return err
	}
	for _, f := range plan.Files {
		if _, err := safeJoin(target, f.Path); err != nil {
			return err
		}
		staged, err := safeJoin(plan.Staging, f.Path)
		if err != nil {
			return err
		}
		if _, err := os.Stat(staged); err != nil {
			return fmt.Errorf("解压文件缺失: %s: %w", f.Path, err)
		}
	}
	return nil
}

// backupOne 把已有旧文件移入事务备份目录。
func backupOne(j *Journal, f FileEntry) error {
	dest, err := safeJoin(j.Target, f.Path)
	if err != nil {
		return err
	}
	backup, err := safeJoin(j.Backup, f.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return err
	}
	return moveWithRetry(dest, backup)
}

// applyOne 把解压文件移入目标位置。
func applyOne(j *Journal, f FileEntry) error {
	staged, err := safeJoin(j.Staging, f.Path)
	if err != nil {
		return err
	}
	dest, err := safeJoin(j.Target, f.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return moveWithRetry(staged, dest)
}

// failAndRollback 回滚并把原始失败与回滚结果一起返回。
func failAndRollback(j *Journal, cause error, report func(Progress)) error {
	rollbackErr := RollbackJournal(j, report)
	if rollbackErr != nil {
		return fmt.Errorf("%w；回滚失败: %v", cause, rollbackErr)
	}
	return fmt.Errorf("%w；已回滚到安装前状态", cause)
}

// finishCommitted 提交后的清理：删除解压目录、剪枝备份、移除事务日志。
func finishCommitted(j *Journal) {
	if withinRoot(stagingRoot(j.Target), j.Staging) {
		if err := os.RemoveAll(j.Staging); err != nil {
			logf("清理解压目录失败（可忽略）: %v", err)
		}
	}
	pruneUpdateBackups(j.Target, backupKeep)
	if err := os.Remove(journalPath(j.Target)); err != nil && !os.IsNotExist(err) {
		logf("移除事务日志失败（可忽略）: %v", err)
	}
}

// pruneUpdateBackups 只保留最近 keep 个备份目录（按名称排序，名称含时间与非ce）。
func pruneUpdateBackups(target string, keep int) {
	if keep <= 0 {
		return
	}
	root := backupRoot(target)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-keep] {
		dir := filepath.Join(root, name)
		if err := os.RemoveAll(dir); err != nil {
			logf("剪枝备份失败（可能包含正在运行的更新器）: %s: %v", dir, err)
		}
	}
}

// withinRoot 判断路径是否位于 root 之下（用于避免误删计划外目录）。
func withinRoot(root, path string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// percent 计算百分比（0-100）。
func percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return done * 100 / total
}

// touch 更新文件修改时间（心跳与时间戳用途）。
func touch(path string) error {
	now := time.Now()
	return os.Chtimes(path, now, now)
}
