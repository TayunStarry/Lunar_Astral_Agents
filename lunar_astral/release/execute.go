package release

import (
	"config"
	"logger"
	"time"
)

// ProcessInfo 结构体用于存储进程信息
type ProcessInfo struct {
	// PID 表示进程的ID
	PID int
	// Port 表示进程占用的端口
	Port int
	// Name 表示进程的名称
	Name string
	// CmdLine 表示启动进程的命令行
	CmdLine string
}

// ExecutePortRelease 函数用于执行端口释放操作，扫描指定范围内的端口，终止占用端口的进程，并验证端口释放情况
func ExecutePortRelease() {
	// 打印提示信息，表明开始扫描端口占用情况
	logger.Info("LunarCore", "正在扫描端口 %d 到 %d 的占用情况...", *config.MinPort, *config.MaxPort)
	// 获取指定端口范围内占用端口的进程列表
	processes := getPortProcessesPowerShell()
	// 如果没有发现占用端口的进程，打印提示信息并等待后返回
	if len(processes) == 0 {
		logger.Info("LunarCore", "端口 %d 到 %d 上未发现任何进程占用", *config.MinPort, *config.MaxPort)
		// 等待 100 毫秒，让系统有时间处理
		time.Sleep(100 * time.Millisecond)
		return
	}
	// 打印发现的占用端口的进程数量
	logger.Info("LunarCore", "发现 %d 个占用端口的进程:", len(processes))
	// 遍历进程列表，打印每个进程的详细信息
	for _, proc := range processes {
		logger.Info("LunarCore", "   - PID: %d, 端口: %d, 进程: %s", proc.PID, proc.Port, proc.Name)
		// 如果进程有命令行信息，打印命令行内容
		if proc.CmdLine != "" {
			logger.Info("LunarCore", "     命令行: %s", proc.CmdLine)
		}
	}
	// 打印提示信息，表明开始终止进程
	logger.Info("LunarCore", "开始终止进程...")
	// 调用 killProcesses 函数终止进程，返回成功终止的进程数量和可能的错误
	killed, err := killProcesses(processes)
	// 如果终止进程时发生错误，打印错误信息
	if err != nil {
		logger.Error("LunarCore", "终止进程时发生错误: %v", err)
	}
	// 等待 500 毫秒，让系统有时间处理进程终止操作
	time.Sleep(500 * time.Millisecond)
	// 打印提示信息，表明开始验证端口释放情况
	logger.Info("LunarCore", "验证端口释放情况:")
	// 调用 verifyPortsQuick 函数验证剩余被占用的端口数量
	remainingPorts := verifyPortsQuick()
	// 根据成功终止的进程数量和剩余被占用的端口数量输出不同的提示信息
	if killed > 0 {
		if remainingPorts == 0 {
			logger.Info("LunarCore", "成功终止了 %d 个进程，所有端口已释放", killed)
		} else {
			logger.Info("LunarCore", "成功终止了 %d 个进程，但仍有 %d 个端口被占用", killed, remainingPorts)
		}
	} else {
		logger.Info("LunarCore", "没有成功终止任何进程")
	}
	// 等待 100 毫秒，让系统有时间处理端口释放
	time.Sleep(100 * time.Millisecond)
}
