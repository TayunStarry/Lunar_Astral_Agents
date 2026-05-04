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
	// 如果配置文件中 EmbeddingModel 字段非空，则更新全局配置
	if parameter.Models.EmbeddingModel != "" {
		*config.EmbeddingModel = parameter.Models.EmbeddingModel
	}
	// 如果配置文件中 MultimodalModel 字段非空，则更新全局配置
	if parameter.Models.MultimodalModel != "" {
		*config.MultimodalModel = parameter.Models.MultimodalModel
	}
	// 如果配置文件中 MmprojModel 字段非空，则更新全局配置
	if parameter.Models.MmprojModel != "" {
		*config.MmprojModel = parameter.Models.MmprojModel
	}
	// 如果配置文件中 DiffusionModel 字段非空，则更新全局配置
	if parameter.Models.DiffusionModel != "" {
		*config.DiffusionModel = parameter.Models.DiffusionModel
	}
	// 如果配置文件中 VariationalModel 字段非空，则更新全局配置
	if parameter.Models.VariationalModel != "" {
		*config.VariationalModel = parameter.Models.VariationalModel
	}
	// 如果配置文件中 PromptRefineModel 字段非空，则更新全局配置
	if parameter.Models.PromptRefineModel != "" {
		*config.PromptRefineModel = parameter.Models.PromptRefineModel
	}
	// 如果配置文件中 TTSUrl 字段非空，则更新全局配置
	if parameter.Server.TTSUrl != "" {
		*config.TTSUrl = parameter.Server.TTSUrl
	}
	// 如果配置文件中 Developer 字段非空，则更新全局配置
	if parameter.Server.Developer == true {
		*config.Developer = true
	} else {
		*config.Developer = false
	}
	// 如果配置文件中 ClearPort 字段非空，则更新全局配置
	if parameter.Server.ClearPort == true {
		*config.ClearPort = true
	} else {
		*config.ClearPort = false
	}
	// 如果配置文件中 AllowDiffusion 字段非空，则更新全局配置
	if parameter.Server.AllowDiffusion == true {
		*config.AllowDiffusion = true
	} else {
		*config.AllowDiffusion = false
	}
	// 如果配置文件中 AllowMultimodal 字段非空，则更新全局配置
	if parameter.Server.AllowMultimodal == true {
		*config.AllowMultimodal = true
	} else {
		*config.AllowMultimodal = false
	}
}
