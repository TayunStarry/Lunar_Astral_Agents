//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// runningProgramProcesses 非 Windows 平台按 /proc/<pid>/exe 匹配，并用信号 0 复核存活。
// 更新器的主要运行平台是 Windows，这里保证可编译、可测试。
func runningProgramProcesses(targets []string) ([]procInfo, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	var out []procInfo
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		link, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
		if err != nil {
			continue
		}
		link = strings.TrimSuffix(link, " (deleted)")
		if matchTarget(link, targets) && processAlive(pid) {
			out = append(out, procInfo{PID: pid, Exe: link})
		}
	}
	return out, nil
}

// processAlive 信号 0 只做存在性检查，不真正投递信号
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}
