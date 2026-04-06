package llama

import (
	"log"
	"open-lunar/browser"
	"open-lunar/parameter"
	"os"
)

// CreateServers 初始化配置，获取模型路径，并启动不同类型的 GGUF 服务实例
func CreateServers() {
	// 初始化配置，若初始化失败则直接返回
	if !initConfig() {
		return
	}
	// 等待加载的模型队列
	modelPaths := map[string]string{}
	// 当配置允许加载推理模型时，将多模态模型加入待加载列表
	if *parameter.AllowMultimodal {
		modelPaths["multimodal"] = *parameter.MultimodalModel
	}
	// 将嵌入模型加入待加载列表
	modelPaths["embedding"] = *parameter.EmbeddingModel
	// 初始化标志位，用于判断是否所有模型路径都为空
	allEmpty := true
	// 若存在非空路径，则更新标志位并跳出循环
	for _, path := range modelPaths {
		if path != "" {
			allEmpty = false
			break
		}
	}
	// 若所有模型路径都为空，输出错误日志提示未找到有效模型文件
	// 同时启动浏览器打开模型下载页面，方便用户下载模型，最后返回
	if allEmpty {
		log.Printf("GGUF模块[ERROR] -> 所有类型均未找到有效模型文件")
		// 启动浏览器页面，辅助用户下载模型
		browser.OpenBrowser("https://modelscope.cn/models/unsloth/Qwen3-VL-30B-A3B-Instruct-GGUF/files")
		browser.OpenBrowser("https://modelscope.cn/models/Qwen/Qwen3-Embedding-0.6B-GGUF/files")
		return
	}
	// 使用获取到的模型路径启动不同类型的 GGUF 服务实例
	startServersWithTypes(modelPaths)
}

// initConfig 初始化配置，创建必要目录
func initConfig() bool {
	// 创建必要目录
	dirs := []string{
		"./local_data",
		"./subsystem",
		"./models",
	}
	// 遍历所有目录，尝试创建它们
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("创建目录失败 %s: %v", dir, err)
			// 任意目录创建失败即返回 false，表示初始化失败
			return false
		}
	}
	// 所有初始化操作成功，返回 true
	return true
}

// startServersWithTypes 为不同类型的模型启动对应的 GGUF 服务实例
func startServersWithTypes(modelPaths map[string]string) {
	// 从配置中获取基础端口号
	basePort := *parameter.ModelPort
	// 遍历所有模型类型和对应的模型路径
	for modelType, modelPath := range modelPaths {
		// 若模型路径为空，记录警告日志并跳过当前循环
		if modelPath == "" {
			log.Printf("GGUF模块[WARNING] -> 类型[%s]未找到模型", modelType)
			continue
		}
		// 增加 最大模型数量
		parameter.MaxModelAmount++
		// 启动单个模型服务
		startServerForModel(modelType, modelPath, basePort)
	}
}
