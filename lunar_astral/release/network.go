package release

import (
	"config"
	"logger"
	"os/exec"
	"strconv"
	"strings"
)

// getPortProcessesNetstat 使用 netstat 命令获取指定端口范围内的进程信息
// 返回一个包含进程信息的 ProcessInfo 切片
func getPortProcessesNetstat() []ProcessInfo {
	// 初始化存储进程信息的切片
	var processes []ProcessInfo
	// 构建 netstat 命令，获取所有 TCP 连接并显示进程 ID
	cmd := exec.Command("netstat", "-ano", "-p", "tcp")
	// 执行命令并获取输出
	output, err := cmd.Output()
	// 检查命令执行是否出错
	if err != nil {
		// 打印错误信息
		logger.Error("LunarCore", "netstat 命令执行失败: %v", err)
		// 出错时返回空切片
		return processes
	}
	// 将命令输出按行分割
	lines := strings.Split(string(output), "\n")
	// 遍历每一行输出
	for _, line := range lines {
		// 去除行首尾的空白字符
		line = strings.TrimSpace(line)
		// 如果行为空或者不包含 "TCP"，则跳过
		if line == "" || !strings.Contains(line, "TCP") {
			continue
		}
		// 将行按空白字符分割成多个字段
		fields := strings.Fields(line)
		// 如果字段数量少于 5 个，说明格式不符合要求，跳过
		if len(fields) < 5 {
			continue
		}
		// 获取连接状态
		state := fields[3]
		// 只处理处于 LISTENING 或 ESTABLISHED 状态的连接
		if state != "LISTENING" && state != "ESTABLISHED" {
			continue
		}
		// 获取本地地址
		localAddr := fields[1]
		// 将本地地址按 ":" 分割
		parts := strings.Split(localAddr, ":")
		// 如果分割后的部分少于 2 个，说明格式不符合要求，跳过
		if len(parts) < 2 {
			continue
		}
		// 将端口号转换为整数
		port, err := strconv.Atoi(parts[len(parts)-1])
		// 检查端口号转换是否成功，以及是否在指定范围内
		if err != nil || port < *config.MinPort || port > *config.MaxPort {
			continue
		}
		// 将进程 ID 转换为整数
		pid, err := strconv.Atoi(fields[4])
		// 检查进程 ID 转换是否成功，以及是否合法
		if err != nil || pid <= 0 {
			continue
		}
		// 根据进程 ID 获取进程名称和命令行
		processName, cmdLine := getProcessInfoPowerShell(pid)
		// 将进程信息添加到切片中
		processes = append(processes, ProcessInfo{
			PID:     pid,
			Port:    port,
			Name:    processName,
			CmdLine: cmdLine,
		})
	}
	// 返回包含所有进程信息的切片
	return processes
}
