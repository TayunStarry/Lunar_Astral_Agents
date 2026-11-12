package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// loadJournal 读取事务日志；不存在时返回 nil, nil。
func loadJournal(target string) (*Journal, error) {
	var j Journal
	if err := readJSONFile(journalPath(target), &j); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if j.Schema != updateSchema {
		return nil, fmt.Errorf("不支持的事务日志版本: %d", j.Schema)
	}
	return &j, nil
}

// saveJournal 原子写入事务日志。
func saveJournal(target string, j *Journal) error {
	return writeJSONAtomic(journalPath(target), j)
}

// setJournalPhase 更新并落盘事务阶段。
func setJournalPhase(target string, j *Journal, phase string) error {
	j.Phase = phase
	return saveJournal(target, j)
}

// buildJournal 依据安装计划生成事务日志（含缺失目录与安装前后状态）。
// 该函数只读目标目录，不产生任何写入。
func buildJournal(plan InstallPlan) (*Journal, error) {
	absTarget, err := filepath.Abs(plan.Target)
	if err != nil {
		return nil, err
	}
	absStaging, err := filepath.Abs(plan.Staging)
	if err != nil {
		return nil, err
	}
	nonce := newNonce()
	backup := plan.Backup
	if backup == "" {
		backup = filepath.Join(backupRoot(absTarget), nonce)
	}

	created, err := missingDirsForTargets(absTarget, plan.Files)
	if err != nil {
		return nil, err
	}

	files := make([]FileEntry, 0, len(plan.Files))
	var total int64
	for _, f := range plan.Files {
		dest, err := safeJoin(absTarget, f.Path)
		if err != nil {
			return nil, err
		}
		info, statErr := os.Stat(dest)
		hadOld := statErr == nil && !info.IsDir()
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		entry := f
		entry.HadOld = hadOld
		files = append(files, entry)
		total += f.Size
	}

	stateBefore, err := readState(absTarget)
	if err != nil {
		return nil, err
	}
	stateAfter := &State{
		Schema:      updateSchema,
		Version:     plan.ToVersion,
		InstalledAt: time.Now().Format(time.RFC3339),
		SourceTag:   plan.ToVersion,
		FileCount:   len(files),
		Bytes:       total,
		LastResult:  "success",
	}

	return &Journal{
		Schema:      updateSchema,
		Nonce:       nonce,
		Phase:       phasePrepared,
		Target:      absTarget,
		FromVersion: plan.FromVersion,
		ToVersion:   plan.ToVersion,
		Staging:     absStaging,
		Backup:      backup,
		CreatedDirs: created,
		Files:       files,
		StateBefore: stateBefore,
		StateAfter:  stateAfter,
	}, nil
}

// missingDirsForTargets 汇总所有目标文件父目录中缺失的目录（去重、由浅到深）。
func missingDirsForTargets(target string, files []FileEntry) ([]string, error) {
	seen := make(map[string]bool)
	var out []string
	for _, f := range files {
		dest, err := safeJoin(target, f.Path)
		if err != nil {
			return nil, err
		}
		missing, err := collectMissingDirs(filepath.Dir(dest))
		if err != nil {
			return nil, err
		}
		for _, dir := range missing {
			if seen[dir] {
				continue
			}
			seen[dir] = true
			out = append(out, dir)
		}
	}
	return out, nil
}
