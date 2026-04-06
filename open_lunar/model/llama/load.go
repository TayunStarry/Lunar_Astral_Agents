package llama

import (
	"Lunar-Astral-Agents/model/llama/metadata"
	"Lunar-Astral-Agents/parameter"
	"bufio"
	"context"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"time"
)

// startServerForModel 为指定模型启动服务进程。
func startServerForModel(modelType, modelPath string, basePort int) {
	// 从模型路径中提取模型名称
	modelName := filepath.Base(modelPath)
	// 计算当前模型服务使用的端口号
	port := basePort + getPortOffset(modelType)
	// 解析模型元数据
	metaData, err := metadata.ParseMetadata(modelPath)
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 模型[ %s ]元数据解析失败: %v", modelName, err)
		return
	}
	// 从元数据中获取模型的上下文长度
	contextLength, ok := metadata.FindFirstMetadataByKeySubstring(metaData, ".context_length").(uint32)
	// 从元数据中获取模型的专家模块数量
	expertCount, expertCountTest := metadata.FindFirstMetadataByKeySubstring(metaData, ".expert_count").(uint32)
	// 创建启动模型服务进程所需的命令行参数
	args := createArgs(modelType, modelName, modelPath, port, contextLength, metaData)
	// 检查创建的命令行参数是否为空，若为空则表示参数创建失败
	if len(args) == 0 {
		// 记录模型参数创建失败的错误信息
		log.Printf("GGUF模块[ERROR] -> 模型[ %s ]创建参数失败", modelName)
		return
	}
	// 检查系统是否处于开发模式
	if *parameter.DevMode {
		// 若处于开发模式，显示模型的所有元数据，方便调试
		metadata.DisplayAllMetadata(modelName, metaData)
	}
	// 如果模型是混合专家(MoE)模型，打印专家模块数量
	if expertCountTest {
		log.Printf("GGUF模块 -> 混合专家(MoE)模型，包含 %d 个专家模块\n", expertCount)
	}
	// 如果未找到上下文长度元数据，记录错误并返回
	if !ok {
		log.Printf("GGUF模块[ERROR] -> 模型[ %s ]未找到 -> 元数据[ context_length : 上下文长度 ]", modelName)
		return
	}
	// 创建执行模型服务进程的命令
	cmd := exec.Command(*parameter.InferEngine, args...)
	// 打开命令的标准输出和标准错误管道
	stdout, stderr := openCmdPipe(cmd)
	if stdout == nil || stderr == nil {
		return
	}
	// 启动模型服务进程
	if err := cmd.Start(); err != nil {
		log.Printf("GGUF模块[ERROR] -> 实例 [ %s ] 启动失败: %v", modelType, err)
		return
	}
	// 记录模型服务实例启动信息
	log.Printf("GGUF模块 -> 实例端口: http://localhost:%d", port)
	// 注册模型类型和对应的端口号
	registerModelPort(modelType, port)
	// 创建一个带有 5 分钟超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	// 确保在函数返回前取消上下文，避免资源泄漏
	defer cancel()
	// 创建 通知模型加载完成的通道 和 接收进程退出错误信息的通道
	modelLoaded, processExited := make(chan bool), make(chan error, 1)
	// 启动一个 goroutine 等待进程退出，并将退出错误信息发送到通道
	go func() {
		err := cmd.Wait()
		processExited <- err
	}()
	// 启动一个 goroutine 监听命令的标准输出和标准错误
	go executeMonitoring(modelName, stdout, stderr, modelLoaded)
	// 使用 select 监听不同通道的事件
	select {
	case <-modelLoaded:
		// 模型加载完成，记录成功信息并增加已就绪模型数量
		log.Printf("GGUF模块 -> 模型[ %s ]成功加载并启动服务", modelName)
		parameter.ModelReady++

	case err := <-processExited:
		if err != nil {
			// 进程异常退出，记录错误信息
			log.Printf("GGUF模块[ERROR] -> 实例 [ %s ] 进程异常退出: %v", modelType, err)
		} else {
			// 进程正常退出，记录信息
			log.Printf("GGUF模块 -> 实例 [ %s ] 进程正常退出", modelType)
		}
		return

	case <-ctx.Done():
		// 模型加载超时，记录超时信息并尝试终止进程
		log.Printf("GGUF模块[ERROR] -> 实例 [ %s ] 模型加载超时", modelType)
		if err := cmd.Process.Kill(); err != nil {
			log.Printf("GGUF模块[ERROR] -> 终止实例 [ %s ] 进程失败: %v", modelType, err)
		}
		return
	}
	// 启动一个 goroutine 等待进程退出
	go waitForProcessExit(cmd, modelType, port)
}

// createArgs 函数用于创建模型服务进程的命令行参数。
func createArgs(modelType, modelName, modelPath string, port int, contextLength uint32, metaData map[string]any) []string {
	// 初始化基础命令行参数
	args := []string{}
	// 根据模型类型添加不同的额外参数
	switch modelType {
	case "embedding":
		// 补全基础模型运行参数
		args = append(args, buildBaseArgs(modelPath, port, contextLength, 4096, 0, 0)...)
		// 为文本嵌入模型添加启用嵌入机制
		args = append(args, "--embeddings")
		// 为默认类型模型添加特定参数，若添加失败则终止函数执行
		if !DefaultModelArgs(&args, modelPath, modelName, metaData) {
			return []string{}
		}

	case "multimodal":
		// 补全基础模型运行参数
		args = append(args, buildBaseArgs(modelPath, port, contextLength, 16384, 1024, 512)...)
		// 为视觉模型添加特定参数，若添加失败则终止函数执行
		if !MultimodalModelArgs(&args, modelPath, modelName, metaData) {
			return []string{}
		}

	default:
		// 补全基础模型运行参数
		args = append(args, buildBaseArgs(modelPath, port, contextLength, 4096, 0, 0)...)
		// 为默认类型模型添加特定参数，若添加失败则终止函数执行
		if !DefaultModelArgs(&args, modelPath, modelName, metaData) {
			return []string{}
		}
	}
	return args
}

// executeMonitoring 函数用于监控模型服务进程的标准输出和标准错误流。
func executeMonitoring(modelName string, stdout io.ReadCloser, stderr io.ReadCloser, modelLoaded chan bool) {
	// 创建 逐行读取标准输出流扫描器 和 逐行读取标准错误流扫描器
	stdoutScanner, stderrScanner := bufio.NewScanner(stdout), bufio.NewScanner(stderr)
	// 启动一个 goroutine 来扫描标准输出流
	go openStdoutScanner(stdoutScanner, modelLoaded)
	// 启动一个 goroutine 来扫描标准错误流
	go openStderrScanner(stderrScanner, modelLoaded, modelName)
}
