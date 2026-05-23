package llama

import (
	"config"
	"log"
	"math"
	"path/filepath"
	"strconv"
)

// buildBaseArgs 构建启动 GGUF 服务的基础命令行参数
func buildBaseArgs(modelPath string, port int, contextLength uint32, maxToken, changeKeep, changeBatch float64) []string {
	// 设置合理的上下文大小上限，考虑到性能与内存的平衡
	ctxSize := int(math.Min(float64(contextLength), maxToken))
	// 计算动态切片大小，确保至少为 16，否则根据上下文大小动态调整
	dynamicSlicing := math.Max(16, float64(ctxSize)/16)
	// 返回基础命令行参数
	return []string{
		// 指定模型文件路径
		"--model", modelPath,
		// 指定服务端口号
		"--port", strconv.Itoa(port),
		// 设置模型上下文大小
		"--ctx-size", strconv.Itoa(ctxSize),
		// 设置模型可以并行处理的请求数量
		"--parallel", "1",
		// 设置模型token批处理块大小
		"--batch-size", strconv.Itoa(int(math.Max(dynamicSlicing, changeBatch))),
		// 缓存复用
		"--cache-reuse", strconv.Itoa(int(math.Max(dynamicSlicing, changeKeep))),
		// 设置保留部分token用于对接聊天记录
		"--keep", strconv.Itoa(int(math.Max(dynamicSlicing*2, changeKeep))),
		// 基础温度
		"--temp", "0.8",
		// 动态温度范围
		"--dynatemp-range", "0.5",
		// 重复惩罚
		"--repeat-penalty", "1.2",
		// 禁用 WebUI
		"--no-webui",
		// 锁定内存，避免交换
		//"--mlock",
		// 启用 Flash Attention 优化，提升注意力机制的计算效率
		"--flash-attn", "on",
		// 启用上下文偏移功能，调整模型处理上下文的方式
		"--context-shift",
		// 禁用模型思考功能
		"--reasoning", "off",
	}
}

// MultimodalModelArgs 为视觉模态模型添加特定的命令行参数
func MultimodalModelArgs(args *[]string, modelPath, modelName string, metadata map[string]any) bool {
	// 获取多模态模型所需的 MMProj 文件路径
	mmprojPath := *config.MmprojModel
	// 若未找到 MMProj 文件，记录错误日志并返回失败
	if mmprojPath == "" {
		log.Printf("GGUF模块[ERROR] -> 视觉模型需要 --mmproj 参数但未找到对应文件: %s", modelPath)
		return false
	}
	// 根据模型元数据、文件大小和可用显存计算块数量和最大安全 GPU 加速层级
	totalLayers, maxSafeLayers, err := getMetadataLayersAndMemory(modelPath, modelName, metadata)
	// 若计算过程中出现错误，记录错误日志并返回失败
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 模型[%s]加载失败: %v", modelName, err)
		return false
	}
	// 向命令行参数中追加 MMProj 文件路径参数
	*args = append(*args, "--mmproj", mmprojPath)
	// 向命令行参数中追加 GPU 加速层级参数
	*args = append(*args, "--n-gpu-layers", strconv.Itoa(maxSafeLayers))
	// 向命令行参数中追加模型规格类型参数
	*args = append(*args, "--spec-type", "draft-mtp")
	// 向命令行参数中追加多模态模型最大请求数量参数
	*args = append(*args, "--spec-draft-n-max", "3")
	// 记录加载视觉模型的信息，包括使用的 MMProj 文件路径和 GPU 加速层级
	log.Printf("GGUF模块 -> 加载视觉模型 [ %s ]", modelName)
	log.Printf("GGUF模块 -> 加载投影模型 [ %s ]", filepath.Base(mmprojPath))
	log.Printf("GGUF模块 -> GPU加速量级: %d/%d", maxSafeLayers, totalLayers)
	// 所有操作成功，返回成功标志
	return true
}

// DefaultModelArgs 为默认类型模型添加特定的命令行参数
func DefaultModelArgs(args *[]string, modelPath, modelName string, metadata map[string]any) bool {
	// 根据模型元数据、文件大小和可用显存计算块数量和最大安全 GPU 加速层级
	totalLayers, maxSafeLayers, err := getMetadataLayersAndMemory(modelPath, modelName, metadata)
	// 若计算过程中出现错误，记录错误日志并返回失败
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 模型[%s]加载失败: %v", modelName, err)
		return false
	}
	// 向命令行参数中追加 GPU 加速层级参数
	*args = append(*args, "--n-gpu-layers", strconv.Itoa(maxSafeLayers))
	// 记录加载模型的信息，包括 GPU 加速层级
	log.Printf("GGUF模块 -> 加载基础模型 [ %s ]", modelName)
	log.Printf("GGUF模块 -> GPU加速量级: %d/%d", maxSafeLayers, totalLayers)
	// 所有操作成功，返回成功标志
	return true
}
