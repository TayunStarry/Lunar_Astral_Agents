package llama

import (
	"LunarCore/config"
	"bufio"
	"io"
	"log"
	"os/exec"
	"strings"
)

// openCmdPipe 函数用于为指定的命令创建标准输出和标准错误管道。
func openCmdPipe(cmd *exec.Cmd) (io.ReadCloser, io.ReadCloser) {
	// 为命令创建标准输出管道
	stdout, err := cmd.StdoutPipe()
	// 若创建标准输出管道失败，记录错误信息并返回 nil
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 创建标准输出管道失败: %v", err)
		return nil, nil
	}
	// 为命令创建标准错误管道
	stderr, err := cmd.StderrPipe()
	// 若创建标准错误管道失败，记录错误信息并返回 nil
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 创建标准错误管道失败: %v", err)
		return nil, nil
	}
	// 返回创建成功的标准输出和标准错误管道
	return stdout, stderr
}

// openStdoutScanner 函数用于扫描模型服务进程的标准输出流。
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
