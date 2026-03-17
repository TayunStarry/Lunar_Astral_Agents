package release

import (
	config "Lunar-Astral-Agents/parameter" // 引入配置模块，用于获取模型路径等配置
	"log"                                  // 用于格式化输入输出
	"strings"                              // 用于字符串操作
	"time"                                 // 用于处理时间相关操作
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
	// 打印提示信息，表明开始执行端口释放操作
	log.Printf("%s", strings.Repeat("-=", 28))
	// 打印提示信息，表明开始扫描端口占用情况
	log.Printf("正在扫描端口 %d 到 %d 的占用情况...\n", *config.MinPort, *config.MaxPort)
	// 获取指定端口范围内占用端口的进程列表
	processes := getPortProcessesPowerShell()
	// 如果没有发现占用端口的进程，打印提示信息并等待后返回
	if len(processes) == 0 {
		log.Printf("端口 %d 到 %d 上未发现任何进程占用\n", *config.MinPort, *config.MaxPort)
		// 等待 100 毫秒，让系统有时间处理
		time.Sleep(100 * time.Millisecond)
		return
	}
	// 打印发现的占用端口的进程数量
	log.Printf("发现 %d 个占用端口的进程:\n", len(processes))
	// 遍历进程列表，打印每个进程的详细信息
	for _, proc := range processes {
		log.Printf("   - PID: %d, 端口: %d, 进程: %s\n", proc.PID, proc.Port, proc.Name)
		// 如果进程有命令行信息，打印命令行内容
		if proc.CmdLine != "" {
			log.Printf("     命令行: %s\n", proc.CmdLine)
		}
	}
	// 打印提示信息，表明开始终止进程
	log.Printf("\n开始终止进程...\n")
	// 调用 killProcesses 函数终止进程，返回成功终止的进程数量和可能的错误
	killed, err := killProcesses(processes)
	// 如果终止进程时发生错误，打印错误信息
	if err != nil {
		log.Printf("终止进程时发生错误: %v\n", err)
	}
	// 等待 500 毫秒，让系统有时间处理进程终止操作
	time.Sleep(500 * time.Millisecond)
	// 打印提示信息，表明开始验证端口释放情况
	log.Printf("\n验证端口释放情况:\n")
	// 调用 verifyPortsQuick 函数验证剩余被占用的端口数量
	remainingPorts := verifyPortsQuick()
	// 根据成功终止的进程数量和剩余被占用的端口数量输出不同的提示信息
	if killed > 0 {
		if remainingPorts == 0 {
			log.Printf("\n成功终止了 %d 个进程，所有端口已释放\n", killed)
		} else {
			log.Printf("\n成功终止了 %d 个进程，但仍有 %d 个端口被占用\n", killed, remainingPorts)
		}
	} else {
		log.Printf("\n没有成功终止任何进程\n")
	}
	// 等待 100 毫秒，让系统有时间处理端口释放
	time.Sleep(100 * time.Millisecond)
}
