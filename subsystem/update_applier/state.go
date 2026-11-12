package main

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// updateRoot 返回程序根目录下的更新器工作区。
func updateRoot(target string) string {
	return filepath.Join(target, updateDirName)
}

// statePath 返回安装状态文件路径。
func statePath(target string) string {
	return filepath.Join(updateRoot(target), updateStateFile)
}

// journalPath 返回事务日志路径。
func journalPath(target string) string {
	return filepath.Join(updateRoot(target), updateJournalFile)
}

// stagingRoot 返回解压子目录根。
func stagingRoot(target string) string {
	return filepath.Join(updateRoot(target), stagingDirName)
}

// backupRoot 返回备份子目录根。
func backupRoot(target string) string {
	return filepath.Join(updateRoot(target), backupDirName)
}

// downloadRoot 返回下载子目录根。
func downloadRoot(target string) string {
	return filepath.Join(updateRoot(target), downloadDirName)
}

// trashRoot 返回回滚暂存子目录根。
func trashRoot(target string) string {
	return filepath.Join(updateRoot(target), trashDirName)
}

// readState 读取安装状态；不存在时返回 nil, nil。
func readState(target string) (*State, error) {
	var st State
	if err := readJSONFile(statePath(target), &st); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &st, nil
}

// writeState 原子写入安装状态。
func writeState(target string, st *State) error {
	return writeJSONAtomic(statePath(target), st)
}

// readVersionBeacon 读取主程序写出的版本信标；缺失或损坏时返回空字符串。
func readVersionBeacon(target string) string {
	var b VersionBeacon
	if err := readJSONFile(filepath.Join(target, versionBeaconRel), &b); err != nil {
		return ""
	}
	return b.Version
}

// currentVersion 返回当前版本：优先版本信标，其次更新器状态，最后空字符串。
func currentVersion(target string) string {
	if v := readVersionBeacon(target); v != "" {
		return v
	}
	if st, err := readState(target); err == nil && st != nil {
		return st.Version
	}
	return ""
}

// newNonce 生成事务标识（时间 + 纳秒尾数，便于排序与排查）。
func newNonce() string {
	return time.Now().Format("20060102-150405.000000")
}
