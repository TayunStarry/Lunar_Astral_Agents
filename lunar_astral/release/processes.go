package release

import (
	"config"
	"encoding/csv"
	"fmt"
	"logger"
	"os/exec"
	"strconv"
	"strings"
)

// commands 定义了用于查询指定端口范围 TCP 连接信息的 PowerShell 命令片段
var commands = []string{
	// 定义端口范围变量
	"$ports = %d..%d",
	// 获取指定端口的 TCP 连接并过滤
	"Get-NetTCPConnection -ErrorAction SilentlyContinue | Where-Object { $ports -contains $_.LocalPort } |",
	// 选择本地端口和所属进程 ID 列
	"Select-Object LocalPort, OwningProcess |",
	// 转换为 CSV 格式，不包含类型信息
	"ConvertTo-Csv -NoTypeInformation",
}

// getPortProcessesPowerShell 使用 PowerShell 命令获取指定端口范围内的进程信息
// 如果执行失败或解析失败，将调用 getPortProcessesNetstat 方法作为备用方案
func getPortProcessesPowerShell() []ProcessInfo {
	// 用于存储获取到的进程信息
	var processes []ProcessInfo
	// 拼接 PowerShell 命令，使用 parameter.MinPort 和 config.MaxPort 填充端口范围
	psCmd := fmt.Sprintf(strings.Join(commands, "\n"), *config.MinPort, *config.MaxPort)
	// 创建执行 PowerShell 命令的命令对象
	cmd := exec.Command("powershell", "-Command", psCmd)
	// 执行命令并获取输出
	output, err := cmd.Output()
	if err != nil {
		// 命令执行失败，打印错误信息并调用备用方法
		logger.Error("LunarCore", "PowerShell 命令执行失败: %v", err)
		return getPortProcessesNetstat()
	}
	// 创建 CSV 读取器，用于解析 PowerShell 命令的输出
	reader := csv.NewReader(strings.NewReader(string(output)))
	// 读取所有 CSV 记录
	records, err := reader.ReadAll()
	if err != nil {
		// 解析失败，打印错误信息和输出内容，并调用备用方法
		logger.Error("LunarCore", "解析 PowerShell 输出失败: %v", err)
		logger.Error("LunarCore", "输出内容: %q", string(output))
		return getPortProcessesNetstat()
	}
	// 如果记录数量少于 2 条（通常第一条是表头），说明没有有效数据，直接返回空列表
	if len(records) < 2 {
		return processes
	}
	// 遍历记录，从第二条开始（跳过表头）
	for i := 1; i < len(records); i++ {
		// 确保记录包含足够的字段
		if len(records[i]) >= 2 {
			// 将本地端口和进程 ID 字符串转换为整数
			port, err1 := strconv.Atoi(records[i][0])
			pid, err2 := strconv.Atoi(records[i][1])
			// 如果转换成功且进程 ID 大于 0
			if err1 == nil && err2 == nil && pid > 0 {
				// 获取进程名称和命令行信息
				processName, cmdLine := getProcessInfoPowerShell(pid)
				// 将进程信息添加到结果列表中
				processes = append(processes, ProcessInfo{
					PID:     pid,
					Port:    port,
					Name:    processName,
					CmdLine: cmdLine,
				})
			}
		}
	}
	// 返回获取到的进程信息列表
	return processes
}
