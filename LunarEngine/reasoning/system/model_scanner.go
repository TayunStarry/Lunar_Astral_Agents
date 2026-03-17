package llama

import (
	config "Lunar-Astral-Agents/parameter" // 导入项目配置模块（如路径、端口等）
	"bufio"                                // 缓冲 I/O 包，用于读取文件内容
	"log"                                  // 标准日志包，用于输出调试/错误信息
	"strings"                              // 字符串操作包，用于字符串处理
)

// openStdoutScanner 函数用于扫描模型服务进程的标准输出流。
// 当模型加载完成时，通过向 modelLoaded 通道发送信号来通知模型加载完成。
// 参数 stdoutScanner 是模型服务进程的标准输出流扫描器。
// 参数 modelLoaded 是一个通道，用于通知模型是否加载完成。
func openStdoutScanner(stdoutScanner *bufio.Scanner, modelLoaded chan bool) {
	// 逐行扫描标准输出内容
	for stdoutScanner.Scan() {
		// 获取当前扫描到的行内容
		line := stdoutScanner.Text()
		// 当已就绪的模型数量达到最大限制，或者当前行包含 "load_backend:" 时，打印系统日志
		if config.ModelReady >= config.MaxModelAmount || strings.Contains(line, "load_backend:") {
			log.Printf("%s", line)
		}
		// 检查输出行是否包含 "starting the main loop"，若包含则表示模型加载完成
		if strings.Contains(line, "starting the main loop") {
			// 向通道发送信号，表示模型已加载完成
			modelLoaded <- true
		}
	}
}

// openStderrScanner 函数用于扫描模型服务进程的标准错误流。
// 当模型加载完成或发生错误时，通过向 modelLoaded 通道发送信号来通知模型加载状态。
// 参数 stderrScanner 是模型服务进程的标准错误流扫描器。
// 参数 modelLoaded 是一个通道，用于通知模型是否加载完成或发生错误。
// 参数 modelName 是当前监控的模型名称，用于记录错误信息。
func openStderrScanner(stderrScanner *bufio.Scanner, modelLoaded chan bool, modelName string) {
	// 逐行扫描标准错误内容
	for stderrScanner.Scan() {
		// 获取当前扫描到的行内容
		line := stderrScanner.Text()
		// 当已就绪的模型数量达到最大限制，或者当前行包含 "load_backend:" 时，打印系统日志
		if config.ModelReady >= config.MaxModelAmount || strings.Contains(line, "load_backend:") {
			log.Printf("%s", line)
		}
		// 检查错误行是否包含 "starting the main loop"，若包含则表示模型加载完成
		if strings.Contains(line, "starting the main loop") {
			// 向通道发送信号，表示模型已加载完成
			modelLoaded <- true
		}
		// 检查错误行是否包含 "error"（不区分大小写），若包含则记录模型运行错误信息
		if strings.Contains(strings.ToLower(line), "error") {
			log.Printf("GGUF模块[ERROR] -> 模型[%s]运行出错: %s", modelName, line)
		}
	}
}
