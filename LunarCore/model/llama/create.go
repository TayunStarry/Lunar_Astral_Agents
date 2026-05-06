package llama

import (
	"browser"
	"config"
	"log"
)

// CreateServers 初始化配置，获取模型路径，并启动不同类型的 GGUF 服务实例
func CreateServers() {
	// 等待加载的模型队列
	modelPaths := map[string]string{}
	// 判断是否在配置中允许加载多模态模型
	if *config.AllowMultimodal == false {
		return
	}
	// 将多模态模型加入待加载列表
	modelPaths["multimodal"] = *config.MultimodalModel
	// 将嵌入模型加入待加载列表
	modelPaths["embedding"] = *config.EmbeddingModel
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
		browser.OpenSystemBrowser("https://modelscope.cn/models/lmstudio-community/Qwen3.5-9B-GGUF/files")
		browser.OpenSystemBrowser("https://modelscope.cn/models/Qwen/Qwen3-Embedding-0.6B-GGUF/files")
		browser.OpenSystemBrowser("https://modelscope.cn/models/unsloth/Z-Image-Turbo-GGUF/files")
		browser.OpenSystemBrowser("https://modelscope.cn/models/unsloth/Qwen3-4B-Instruct-2507-GGUF/files")
		browser.OpenSystemBrowser("https://modelscope.cn/models/Tongyi-MAI/Z-Image-Turbo/tree/master/vae")
		return
	}
	// 使用获取到的模型路径启动不同类型的 GGUF 服务实例
	startServersWithTypes(modelPaths)
}

// startServersWithTypes 为不同类型的模型启动对应的 GGUF 服务实例
func startServersWithTypes(modelPaths map[string]string) {
	// 从配置中获取基础端口号
	basePort := *config.ModelPort
	// 遍历所有模型类型和对应的模型路径
	for modelType, modelPath := range modelPaths {
		// 若模型路径为空，记录警告日志并跳过当前循环
		if modelPath == "" {
			log.Printf("GGUF模块[WARNING] -> 类型[%s]未找到模型", modelType)
			continue
		}
		// 增加 最大模型数量
		config.MaxModelAmount++
		// 启动单个模型服务
		startServerForModel(modelType, modelPath, basePort)
	}
}
