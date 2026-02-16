package release

import (
	"fmt"     // 用于格式化输入输出
	"log"     // 用于打印日志信息
	"os/exec" // 用于执行外部命令
	"strconv" // 用于字符串到整数的转换
	"time"    // 用于处理时间相关操作
)

// killProcesses 用于终止传入的进程信息列表中的所有进程，并返回成功终止的进程数量。若终止过程中出现错误，会返回当前已终止的数量和错误信息。
// 参数 processes: 包含待终止进程信息的 ProcessInfo 切片。
// 返回值: 成功终止的进程数量和可能出现的错误。
func killProcesses(processes []ProcessInfo) (int, error) {
	// 记录成功终止的进程数量
	killed := 0
	// 遍历进程信息列表
	for _, proc := range processes {
		// 打印正在终止的进程信息
		log.Printf("正在终止进程: PID %d (%s) - 占用端口 %d\n", proc.PID, proc.Name, proc.Port)
		// 创建用于终止指定 PID 进程的命令，/f 表示强制终止
		cmd := exec.Command("taskkill", "/f", "/pid", strconv.Itoa(proc.PID))
		// 执行命令以终止进程
		err := cmd.Run()
		// 检查命令执行是否出错
		if err != nil {
			// 打印终止失败的信息
			log.Printf("   无法终止进程 %d: %v\n", proc.PID, err)
			// 返回当前已终止的进程数量和错误信息
			return killed, fmt.Errorf("终止进程 %d 失败: %v", proc.PID, err)
		} else {
			// 终止成功，增加计数
			killed++
			// 打印终止成功的信息
			log.Printf("   成功终止进程 %d\n", proc.PID)
		}
		// 每个进程终止后暂停 100 毫秒，避免操作过于频繁
		time.Sleep(100 * time.Millisecond)
	}
	// 返回成功终止的进程数量，无错误
	return killed, nil
}
