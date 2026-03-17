package utils

// 引入必要的包
import (
	"log"     // 用于记录日志信息
	"os/exec" // 用于执行外部命令
	"runtime" // 用于获取运行时的系统信息
)

// OpenBrowser 函数用于在不同操作系统上打开指定 URL 的浏览器
func OpenBrowser(url string) {
	// 定义要执行的命令名称
	var cmd string
	// 定义命令的参数列表
	var args []string
	// 根据不同的操作系统选择对应的浏览器打开命令
	switch runtime.GOOS {
	case "windows":
		// Windows 系统下使用 cmd 命令，/c 表示执行完命令后关闭命令行窗口，start 用于打开指定 URL
		cmd = "cmd"
		args = []string{"/c", "start", url}

	case "darwin":
		// macOS 系统下使用 open 命令打开指定 URL
		cmd = "open"
		args = []string{url}

	default:
		// 其他类 Unix 系统（如 Linux）使用 xdg-open 命令打开指定 URL
		cmd = "xdg-open"
		args = []string{url}
	}

	// 执行命令尝试打开浏览器
	if err := exec.Command(cmd, args...).Start(); err != nil {
		// 若打开失败，记录错误日志并提示手动访问
		log.Printf("Web服务[ERROR] -> %v 建议手动访问 : %s", err, url)
	}
}
