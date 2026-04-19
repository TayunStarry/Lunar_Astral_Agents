package server

import (
	config "LunarCore/config"
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
	// 拼接配置文件 lunar_config.json 的完整路径
	configPath := filepath.Join(exeDir, *config.LocalDir, "lunar_config.json")
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
	// 如果配置文件中 Embedding 字段非空，则更新全局配置
	if parameter.Models.Embedding != "" {
		*config.EmbeddingModel = parameter.Models.Embedding
	}
	// 如果配置文件中 Multimodal 字段非空，则更新全局配置
	if parameter.Models.Multimodal != "" {
		*config.MultimodalModel = parameter.Models.Multimodal
	}
	// 如果配置文件中 MultimodalMmproj 字段非空，则更新全局配置
	if parameter.Models.MultimodalMmproj != "" {
		*config.MmprojModel = parameter.Models.MultimodalMmproj
	}
	// 如果配置文件中 Diffusion 字段非空，则更新全局配置
	if parameter.Models.Diffusion != "" {
		*config.DiffusionModel = parameter.Models.Diffusion
	}
	// 如果配置文件中 Variational 字段非空，则更新全局配置
	if parameter.Models.Variational != "" {
		*config.VariationalModel = parameter.Models.Variational
	}
	// 如果配置文件中 PromptRefine 字段非空，则更新全局配置
	if parameter.Models.PromptRefine != "" {
		*config.PromptModel = parameter.Models.PromptRefine
	}
}
