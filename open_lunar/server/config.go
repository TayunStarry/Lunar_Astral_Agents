package server

import (
	config "Lunar-Astral-Agents/parameter"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// loadConfigureFile 加载配置文件
func loadConfigureFile() {
	// 获取当前可执行文件的路径
	exePath, err := os.Executable()
	// 若获取失败，打印错误日志并直接返回
	if err != nil {
		log.Printf("获取可执行文件路径失败: %v", err)
		return
	}
	// 提取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)
	// 拼接配置文件 model_config.json 的完整路径
	configPath := filepath.Join(exeDir, *config.LocalDir, "model_config.json")
	// 读取配置文件内容
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 若读取失败，打印错误日志并直接返回
		log.Printf("读取配置文件失败 %s: %v", configPath, err)
		return
	}
	// 创建 ModelConfig 结构体实例用于接收解析结果
	parameter := &ModelConfig{}
	// 将 JSON 数据解析到结构体中
	if err := json.Unmarshal(data, parameter); err != nil {
		// 若解析失败，打印错误日志并直接返回
		log.Printf("解析配置文件失败: %v", err)
		return
	}
	// 如果配置文件中 EmbeddingModelPath 字段非空，则更新全局配置
	if parameter.EmbeddingModelPath != "" {
		*config.EmbeddingModel = parameter.EmbeddingModelPath
	}
	// 如果配置文件中 MultimodalModelPath 字段非空，则更新全局配置
	if parameter.MultimodalModelPath != "" {
		*config.MultimodalModel = parameter.MultimodalModelPath
	}
	// 如果配置文件中 MmprojModelPath 字段非空，则更新全局配置
	if parameter.MmprojModelPath != "" {
		*config.MmprojModel = parameter.MmprojModelPath
	}
	// 如果配置文件中 DiffusionModelPath 字段非空，则更新全局配置
	if parameter.DiffusionModelPath != "" {
		*config.DiffusionModel = parameter.DiffusionModelPath
	}
	// 如果配置文件中 VariationalModelPath 字段非空，则更新全局配置
	if parameter.VariationalModelPath != "" {
		*config.VariationalModel = parameter.VariationalModelPath
	}
	// 如果配置文件中 PromptRefineModelPath 字段非空，则更新全局配置
	if parameter.PromptRefineModelPath != "" {
		*config.PromptModel = parameter.PromptRefineModelPath
	}
}
