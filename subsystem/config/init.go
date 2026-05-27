package config

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
)

// ModelConfig 定义模型配置的结构
type ModelConfig struct {
	// 模型配置
	Models struct {
		// 扩散模型路径
		DiffusionModel string `json:"diffusion_model"`
		// 变分模型路径
		VariationalModel string `json:"variational_model"`
		// 提示精炼模型路径
		PromptRefineModel string `json:"prompt_refine_model"`
		// ASR模型路径
		AsrModel string `json:"asr_model"`
	} `json:"models"`
	// 服务器配置
	Server struct {
		// 是否为开发者模式
		Developer bool `json:"developer"`
		// 是否清除端口
		ClearPort bool `json:"clear_port"`
		// 是否允许加载扩散模型
		AllowDiffusion bool `json:"allow_diffusion"`
		// 是否允许加载多模态模型
		AllowMultimodal bool `json:"allow_multimodal"`
	} `json:"server"`
	// 云模型配置
	Cloud struct {
		// 云模型服务地址
		CloudModelUrl string `json:"cloud_model_url"`
	} `json:"cloud"`
}

// init 加载配置文件
func init() {
	// 解析命令行参数
	flag.Parse()
	// 获取当前可执行文件的路径
	exePath, err := os.Executable()
	// 若获取失败，打印错误日志并直接返回
	if err != nil {
		log.Printf("[Config][ERROR] -> 获取可执行文件路径失败: %v", err)
		return
	}
	// 提取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)
	// 拼接配置文件 lunar_config.json 的完整路径
	configPath := filepath.Join(exeDir, *LocalDir, "lunar_config.json")
	// 读取配置文件内容
	data, err := os.ReadFile(configPath)
	if err != nil {
		// 若读取失败，打印错误日志并直接返回
		log.Printf("[Config][ERROR] -> 读取配置文件失败 %s: %v", configPath, err)
		return
	}
	// 创建 ModelConfig 结构体实例用于接收解析结果
	parameter := &ModelConfig{}
	// 将 JSON 数据解析到结构体中
	if err := json.Unmarshal(data, parameter); err != nil {
		// 若解析失败，打印错误日志并直接返回
		log.Printf("[Config][ERROR] -> 解析配置文件失败: %v", err)
		return
	}
	// 如果配置文件中 DiffusionModel 字段非空，则更新全局配置
	if parameter.Models.DiffusionModel != "" {
		*DiffusionModel = parameter.Models.DiffusionModel
	}
	// 如果配置文件中 VariationalModel 字段非空，则更新全局配置
	if parameter.Models.VariationalModel != "" {
		*VariationalModel = parameter.Models.VariationalModel
	}
	// 如果配置文件中 PromptRefineModel 字段非空，则更新全局配置
	if parameter.Models.PromptRefineModel != "" {
		*PromptRefineModel = parameter.Models.PromptRefineModel
	}
	// 如果配置文件中 AsrModel 字段非空，则更新全局配置
	if parameter.Models.AsrModel != "" {
		*AsrModel = parameter.Models.AsrModel
	}
	// 如果配置文件中 CloudModelUrl 字段非空，则更新全局配置
	if parameter.Cloud.CloudModelUrl != "" {
		*CloudModelUrl = parameter.Cloud.CloudModelUrl
	}
	// 如果配置文件中 Developer 字段非空，则更新全局配置
	if parameter.Server.Developer == true {
		*Developer = true
	} else {
		*Developer = false
	}
	// 如果配置文件中 ClearPort 字段非空，则更新全局配置
	if parameter.Server.ClearPort == true {
		*ClearPort = true
	} else {
		*ClearPort = false
	}
	// 如果配置文件中 AllowDiffusion 字段非空，则更新全局配置
	if parameter.Server.AllowDiffusion == true {
		*AllowDiffusion = true
	} else {
		*AllowDiffusion = false
	}
	// 如果配置文件中 AllowMultimodal 字段非空，则更新全局配置
	if parameter.Server.AllowMultimodal == true {
		*AllowMultimodal = true
	} else {
		*AllowMultimodal = false
	}
}
