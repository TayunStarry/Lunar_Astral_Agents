package main

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ExecutePortRelease 执行端口释放操作：扫描指定范围内的端口，终止占用端口的进程，并验证端口释放情况
func ExecutePortRelease(portRange PortRange) error {
	fmt.Printf("正在扫描端口 %d 到 %d 的占用情况...\n", portRange.Start, portRange.End)

	// 获取端口范围内占用端口的进程列表
	processes := getPortProcesses(portRange)
	if len(processes) == 0 {
		fmt.Printf("端口 %d 到 %d 上未发现任何进程占用\n", portRange.Start, portRange.End)
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	fmt.Printf("发现 %d 个占用端口的进程:\n", len(processes))
	for _, proc := range processes {
		fmt.Printf("  - PID: %d, 端口: %d, 进程: %s\n", proc.PID, proc.Port, proc.Name)
		if proc.CmdLine != "" {
			fmt.Printf("    命令行: %s\n", proc.CmdLine)
		}
	}

	fmt.Println("开始终止进程...")
	killed, err := killProcesses(processes)
	if err != nil {
		fmt.Printf("  [ERROR] 终止进程时发生错误: %v\n", err)
	}

	time.Sleep(500 * time.Millisecond)

	fmt.Println("验证端口释放情况:")
	remainingPorts := verifyPorts(portRange)

	if killed > 0 {
		if remainingPorts == 0 {
			fmt.Printf("成功终止了 %d 个进程，所有端口已释放\n", killed)
		} else {
			fmt.Printf("成功终止了 %d 个进程，但仍有 %d 个端口被占用\n", killed, remainingPorts)
		}
	} else {
		fmt.Println("没有成功终止任何进程")
	}

	time.Sleep(100 * time.Millisecond)
	return nil
}

// getPortProcesses 使用 PowerShell 获取指定端口范围内的进程信息
// 如果执行失败，回退到 netstat 方式
func getPortProcesses(portRange PortRange) []ProcessInfo {
	var processes []ProcessInfo

	// 首选 PowerShell 方式
	psCmd := fmt.Sprintf(strings.Join(powershellCommands, "\n"), portRange.Start, portRange.End)
	cmd := exec.Command("powershell", "-Command", psCmd)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("  [WARN] PowerShell 命令执行失败，回退到 netstat: %v\n", err)
		return getPortProcessesNetstat(portRange)
	}

	reader := csv.NewReader(strings.NewReader(string(output)))
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("  [WARN] 解析 PowerShell 输出失败，回退到 netstat: %v\n", err)
		return getPortProcessesNetstat(portRange)
	}

	if len(records) < 2 {
		return processes
	}

	for i := 1; i < len(records); i++ {
		if len(records[i]) >= 2 {
			port, err1 := strconv.Atoi(records[i][0])
			pid, err2 := strconv.Atoi(records[i][1])
			if err1 == nil && err2 == nil && pid > 0 {
				processName, cmdLine := getProcessInfo(pid)
				processes = append(processes, ProcessInfo{
					PID:     pid,
					Port:    port,
					Name:    processName,
					CmdLine: cmdLine,
				})
			}
		}
	}
	return processes
}

// getPortProcessesNetstat 使用 netstat 获取端口进程信息（备用方案）
func getPortProcessesNetstat(portRange PortRange) []ProcessInfo {
	var processes []ProcessInfo

	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("  [ERROR] netstat 命令执行失败: %v\n", err)
		return processes
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "TCP") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		state := fields[3]
		if state != "LISTENING" && state != "ESTABLISHED" {
			continue
		}

		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) < 2 {
			continue
		}

		port, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil || port < portRange.Start || port > portRange.End {
			continue
		}

		pid, err := strconv.Atoi(fields[4])
		if err != nil || pid <= 0 {
			continue
		}

		processName, cmdLine := getProcessInfo(pid)
		processes = append(processes, ProcessInfo{
			PID:     pid,
			Port:    port,
			Name:    processName,
			CmdLine: cmdLine,
		})
	}
	return processes
}

// getProcessInfo 通过 PowerShell 获取指定进程的名称和命令行
func getProcessInfo(pid int) (string, string) {
	// 获取进程名
	nameCmd := exec.Command("powershell", "-Command",
		fmt.Sprintf("(Get-Process -Id %d -ErrorAction SilentlyContinue).ProcessName", pid))
	nameOutput, err := nameCmd.Output()
	processName := "未知进程"
	if err == nil {
		name := strings.TrimSpace(string(nameOutput))
		if name != "" {
			processName = name
		}
	}

	// 获取命令行
	cmdLineCmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`(Get-WmiObject Win32_Process -Filter "ProcessId = %d").CommandLine`, pid))
	cmdLineOutput, err := cmdLineCmd.Output()
	cmdLine := ""
	if err == nil {
		cmdLine = strings.TrimSpace(string(cmdLineOutput))
	}

	return processName, cmdLine
}

// killProcesses 终止进程列表中的所有进程
func killProcesses(processes []ProcessInfo) (int, error) {
	killed := 0
	for _, proc := range processes {
		fmt.Printf("正在终止进程: PID %d (%s) - 占用端口 %d\n", proc.PID, proc.Name, proc.Port)
		cmd := exec.Command("taskkill", "/f", "/pid", strconv.Itoa(proc.PID))
		err := cmd.Run()
		if err != nil {
			fmt.Printf("  [ERROR] 无法终止进程 %d: %v\n", proc.PID, err)
			return killed, fmt.Errorf("终止进程 %d 失败: %w", proc.PID, err)
		}
		killed++
		fmt.Printf("  ✓ 成功终止进程 %d\n", proc.PID)
		time.Sleep(100 * time.Millisecond)
	}
	return killed, nil
}

// verifyPorts 验证端口范围内的端口释放情况，返回仍被占用的端口数
func verifyPorts(portRange PortRange) int {
	remaining := 0
	for port := portRange.Start; port <= portRange.End; port++ {
		if isPortInUse(port) {
			fmt.Printf("  端口 %d 仍被占用\n", port)
			remaining++
		} else {
			fmt.Printf("  端口 %d 已释放\n", port)
		}
	}
	return remaining
}

// isPortInUse 检查指定端口是否被占用
func isPortInUse(port int) bool {
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	portStr := fmt.Sprintf(":%d", port)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, portStr) &&
			(strings.Contains(line, "LISTENING") || strings.Contains(line, "ESTABLISHED")) {
			return true
		}
	}
	return false
}