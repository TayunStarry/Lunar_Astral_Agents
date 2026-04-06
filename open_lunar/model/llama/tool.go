// 声明包名为 gguf
package llama

import (
	"Lunar-Astral-Agents/parameter"
	"log"
	"os/exec"
)

// registerModelPort 函数用于注册模型端口到全局映射
func registerModelPort(modelType string, port int) {
	// 加锁，保证对全局映射的并发安全访问
	parameter.ModelMapMutex.Lock()
	// 函数返回前自动解锁
	defer parameter.ModelMapMutex.Unlock()
	// 将模型类型添加 "system-" 前缀后作为键，端口号作为值存储到全局映射中
	parameter.ModelPortMap["system-"+modelType] = port
}

// waitForProcessExit 函数用于等待命令对应的进程退出，并在进程异常退出时记录错误日志
func waitForProcessExit(cmd *exec.Cmd, modelType string, port int) {
	// 等待命令对应的进程退出
	if err := cmd.Wait(); err != nil {
		// 若进程异常退出，记录错误日志，包含模型类型、端口号和错误信息
		log.Printf("GGUF模块[ERROR] -> 实例 %s (端口 %d) 退出: %v", modelType, port, err)
	}
}

// getPortOffset 函数用于获取模型类型对应的端口号偏移量
func getPortOffset(modelType string) int {
	// 根据不同的模型类型返回对应的端口偏移量
	switch modelType {
	case "embedding":
		// 文本嵌入模型，端口偏移量为 0
		return 0
	case "reasoning":
		// 推理模型，端口偏移量为 1
		return 1
	case "multimodal":
		// 多模态模型，端口偏移量为 2
		return 2
	default:
		// 未知类型模型，默认端口偏移量为 9
		return 9
	}
}
