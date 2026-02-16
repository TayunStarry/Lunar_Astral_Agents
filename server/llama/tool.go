// 声明包名为 gguf
package llama

// 导入必要的包
import (
	"Lunar-Astral-Agents/server/config" // 导入项目配置模块（如路径、端口等）
	"log"                               // 标准日志包，用于输出调试/错误信息
	"os/exec"                           // 执行外部命令（启动 GGUF 服务进程）
)

/**
 * @description: 注册模型端口到全局映射
 * @description: 会对模型类型添加 "system-" 前缀后作为键，端口号作为值存储到映射中
 * @description: 为保证并发安全，操作前会对映射加锁，操作完成后自动解锁
 * @param {string} modelType 模型的类型，如 "embedding", "reasoning", "visual" 等
 * @param {int} port 该模型对应的服务端口号
 * @return {*} 无返回值
 */
func registerModelPort(modelType string, port int) {
	// 加锁，保证对全局映射的并发安全访问
	config.ModelMapMutex.Lock()
	// 函数返回前自动解锁
	defer config.ModelMapMutex.Unlock()
	// 将模型类型添加 "system-" 前缀后作为键，端口号作为值存储到全局映射中
	config.ModelPortMap["system-"+modelType] = port
}

/**
 * @description: 等待命令对应的进程退出，并在进程异常退出时记录错误日志
 * @param {*exec.Cmd} cmd 要等待退出的命令对象
 * @param {string} modelType 模型类型，如 "embedding", "reasoning", "visual" 等
 * @param {int} port 该模型对应的服务端口号
 * @return {*} 无返回值
 */
func waitForProcessExit(cmd *exec.Cmd, modelType string, port int) {
	// 等待命令对应的进程退出
	if err := cmd.Wait(); err != nil {
		// 若进程异常退出，记录错误日志，包含模型类型、端口号和错误信息
		log.Printf("GGUF模块[ERROR] -> 实例 %s (端口 %d) 退出: %v", modelType, port, err)
	}
}

/**
 * @description: 获取模型类型对应的端口号偏移量
 * @param {string} modelType 模型的类型，如 "embedding", "reasoning", "visual" 等
 * @return {int} 模型类型对应的端口号偏移量
 */
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
