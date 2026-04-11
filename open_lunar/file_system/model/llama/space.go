// 声明包名为 gguf
package llama

import (
	"fmt"
	"log"
	"math"
	"open-lunar/file_system/model/llama/metadata"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// GetFreeMemory 函数用于获取当前系统中所有 GPU 的空闲显存（MB）
func GetFreeMemory() (uint64, error) {
	// 创建执行 nvidia-smi 命令的对象，查询空闲显存并以无表头、无单位的 CSV 格式输出
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.free", "--format=csv,noheader,nounits")
	// 执行命令并获取输出结果
	output, err := cmd.CombinedOutput()
	// 若命令执行失败（如未安装 nvidia-smi、驱动异常等），返回错误
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi 执行失败: %w", err)
	}
	// 去除输出结果的前后空白字符并按行分割
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	// 如果没有返回任何行数据，说明没有检测到 GPU 或命令无输出
	if len(lines) == 0 {
		return 0, fmt.Errorf("未检测到 GPU 信息")
	}
	// 解析第一行输出为无符号整数，表示空闲显存大小（MB）
	freeMemMB, err := strconv.ParseUint(strings.TrimSpace(lines[0]), 10, 64)
	// 解析失败（如非数字、格式错误）时返回错误
	if err != nil {
		return 0, fmt.Errorf("显存值解析失败: %w", err)
	}
	// 将空闲显存大小从 MB 转换为字节并返回
	return freeMemMB * 1024 * 1024, nil
}

// getFileInfoAndMemory 函数用于获取模型文件的信息和可用显存
func getFileInfoAndMemory(modelPath, modelName string) (os.FileInfo, uint64, error) {
	// 打印模型加载分隔符
	log.Printf("%s", strings.Repeat("-=", 28))
	// 获取模型文件的信息
	fileInfo, err := os.Stat(modelPath)
	// 若获取文件信息失败，记录错误日志并返回错误
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 模型[%s]加载失败: %v", modelName, err)
		return nil, 0, err
	}
	// 获取可用显存大小
	freeMem, err := GetFreeMemory()
	// 若获取显存信息失败，记录错误日志并使用默认值 8GB
	if err != nil {
		log.Printf("GGUF模块[ERROR] -> 显存检测失败，使用默认值 2GB: %v", err)
		freeMem = 2 * 1024 * 1024 * 1024
	} else {
		// 若获取显存信息成功，记录检测到的可用显存大小
		log.Printf("GGUF模块 -> 检测到可用显存: %d MB", freeMem/1024/1024)
	}
	// 返回模型文件信息、可用显存大小和 nil 错误
	return fileInfo, freeMem, nil
}

// calculateMetadataLayers 函数用于根据模型元数据、文件大小和可用显存计算最大安全 GPU 加速层级
func calculateMetadataLayers(metaData map[string]any, fileSize float64, freeMem uint64) (totalLayers, maxSafeLayers int, err error) {
	// 从元数据中获取 .block_count 键对应的值
	val, ok := metadata.FindFirstMetadataByKeySubstring(metaData, ".block_count").(uint32)
	// 检查元数据是否存在
	if !ok {
		return 0, 0, fmt.Errorf("GGUF模块[ERROR] -> 元数据[ block_count : GPU卸载层数 ]不存在")
	}
	// 转换为 int 类型
	totalLayers = int(val)
	// 检查 totalLayers 是否为 0
	if totalLayers == 0 {
		return 0, 0, fmt.Errorf("GGUF模块[ERROR] -> 元数据[ block_count : GPU卸载层数 ]值为 0")
	}
	// 预留显存，默认 2GB
	reserveMem := uint64(2 * 1024 * 1024 * 1024)
	// 计算每层的大小，考虑激活值内存（1.75 倍）
	layerSize := (fileSize / float64(totalLayers)) * 1.75
	// 计算可用的层数，考虑显存预留
	maxLayersByMemory := math.Abs(float64(freeMem-reserveMem)) / layerSize
	// 确保在有效范围内
	safeLayers := int(math.Max(0, math.Min(float64(totalLayers), math.Floor(maxLayersByMemory))))
	// 返回计算结果
	return totalLayers, safeLayers, nil
}

// getMetadataLayersAndMemory 函数用于获取模型文件的元数据信息和可用显存，计算最大安全 GPU 加速层级
func getMetadataLayersAndMemory(modelPath, modelName string, metadata map[string]any) (totalLayers, maxSafeLayers int, err error) {
	// 获取模型文件信息和可用显存
	fileInfo, freeMem, err := getFileInfoAndMemory(modelPath, modelName)
	if err != nil {
		return 0, 0, err
	}
	// 根据模型元数据、文件大小和可用显存计算块数量和最大安全 GPU 加速层级
	return calculateMetadataLayers(metadata, float64(fileInfo.Size()), freeMem)
}
