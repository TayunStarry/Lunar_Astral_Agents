package release

import (
	"fmt"
	"log"
	"open-lunar/parameter"
	"os/exec"
	"strings"
)

// getProcessInfoPowerShell 通过 PowerShell 获取指定进程 ID 的进程名和命令行参数
// 参数:
//   - pid: 进程的 ID
//
// 返回值:
//   - string: 进程名，若获取失败则返回 "未知进程"
//   - string: 进程的命令行参数，若获取失败则返回空字符串
func getProcessInfoPowerShell(pid int) (string, string) {
	// 构建获取进程名的 PowerShell 命令
	nameCmd := exec.Command("powershell", "-Command", fmt.Sprintf("(Get-Process -Id %d -ErrorAction SilentlyContinue).ProcessName", pid))
	// 执行命令并获取输出
	nameOutput, err := nameCmd.Output()
	// 默认进程名为 "未知进程"
	processName := "未知进程"
	// 若命令执行成功
	if err == nil {
		// 去除输出的前后空白字符
		name := strings.TrimSpace(string(nameOutput))
		// 若输出不为空，则更新进程名
		if name != "" {
			processName = name
		}
	}
	// 构建获取进程命令行参数的 PowerShell 命令
	cmdLineCmd := exec.Command("powershell", "-Command", fmt.Sprintf(`(Get-WmiObject Win32_Process -Filter "ProcessId = %d").CommandLine`, pid))
	// 执行命令并获取输出
	cmdLineOutput, err := cmdLineCmd.Output()
	// 默认命令行参数为空字符串
	cmdLine := ""
	// 若命令执行成功
	if err == nil {
		// 去除输出的前后空白字符
		cmdLine = strings.TrimSpace(string(cmdLineOutput))
	}
	// 返回进程名和命令行参数
	return processName, cmdLine
}

// verifyPortsQuick 快速验证从 startPort 到 endPort 范围内的端口使用情况
// 返回仍被占用的端口数量
func verifyPortsQuick() int {
	// 记录仍被占用的端口数量
	remaining := 0
	// 遍历从 config.MinPort 到 config.MaxPort 的所有端口
	for port := *parameter.MinPort; port <= *parameter.MaxPort; port++ {
		// 检查端口是否被占用
		if isPortInUse(port) {
			// 若端口被占用，打印提示信息
			log.Printf("   端口 %d 仍被占用\n", port)
			remaining++
		} else {
			// 若端口未被占用，打印提示信息
			log.Printf("   端口 %d 已释放\n", port)
		}
	}
	return remaining
}

// isPortInUse 检查指定端口是否被占用
// 参数:
//   - port: 要检查的端口号
//
// 返回值:
//   - bool: 若端口被占用返回 true，否则返回 false
func isPortInUse(port int) bool {
	// 构建 netstat 命令，用于获取 TCP 连接信息
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	// 执行命令并获取输出
	output, err := cmd.Output()
	// 若命令执行失败，默认端口未被占用
	if err != nil {
		return false
	}
	// 按行分割命令输出
	lines := strings.Split(string(output), "\n")
	// 构建端口匹配字符串
	portStr := fmt.Sprintf(":%d", port)
	// 遍历每一行输出
	for _, line := range lines {
		// 若当前行包含端口号，并且状态为 LISTENING 或 ESTABLISHED
		if strings.Contains(line, portStr) &&
			(strings.Contains(line, "LISTENING") || strings.Contains(line, "ESTABLISHED")) {
			return true
		}
	}
	return false
}
