//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stillActive GetExitCodeProcess 的「进程仍在运行」返回值
const stillActive = 259

// runningProgramProcesses 枚举进程，按可执行文件完整路径匹配 targets，
// 再用 PID + 退出码复核存活。返回空切片表示确认没有目标进程在运行。
func runningProgramProcesses(targets []string) ([]procInfo, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	// 先按 exe 基本名预筛，避免为每个进程都打开句柄查询完整路径
	names := make(map[string]bool, len(targets))
	for _, t := range targets {
		if t != "" {
			names[strings.ToLower(filepath.Base(t))] = true
		}
	}

	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	self := os.Getpid()
	var out []procInfo
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}
	for {
		pid := int(entry.ProcessID)
		base := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		if pid != self && base != "" && names[base] {
			full := processImagePath(uint32(pid))
			matched := false
			if full != "" {
				matched = matchTarget(full, targets)
			} else {
				// 路径读不到（权限不足）时按同名视为命中：宁可等待，也不误跳过
				matched = true
			}
			if matched && processAlive(uint32(pid)) {
				out = append(out, procInfo{PID: pid, Exe: full})
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out, nil
}

// processImagePath 查询进程可执行文件完整路径（读不到时返回空串）
func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// processAlive PID + 退出码双重确认：打开失败或已退出都视为不在运行
func processAlive(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
