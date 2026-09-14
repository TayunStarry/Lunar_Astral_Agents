package GeneralConfig

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ModelConfig 定义模型配置的结构
type ModelConfig struct {
	// 模型配置
	Models struct {
		DiffusionModel      string `json:"diffusion_model"`       // 扩散模型路径
		VariationalModel    string `json:"variational_model"`     // 变分模型路径
		PromptAnalysisModel string `json:"prompt_analysis_model"` // 提示分析模型路径
		RealESRGANModel     string `json:"real_esrgan_model"`     // 4x超分辨率模型路径
		AsrModel            string `json:"asr_model"`             // ASR模型路径
	} `json:"models"`
	// 服务器配置
	Server struct {
		Developer      bool `json:"developer"`       // 是否为开发者模式
		AllowDiffusion bool `json:"allow_diffusion"` // 是否允许加载扩散模型
		// 画图前显存守卫：使用指针以便区分"未配置"与"显式关闭"，未配置时保持默认值（开启，阈值 8192 MiB）
		ImageVRAMGuard    *bool `json:"image_vram_guard,omitempty"`
		ImageVRAMGuardMiB *int  `json:"image_vram_guard_mib,omitempty"`
		// sd.cpp 权重内存卸载模式（auto/always/off）与提示词编码器 CPU 运行开关
		SDOffloadToCPU     *string `json:"sd_offload_to_cpu,omitempty"`
		SDTextEncoderOnCPU *bool   `json:"sd_te_on_cpu,omitempty"`
		// 是否允许局域网访问引擎 HTTP/WS 服务：未配置默认仅绑定 127.0.0.1 回环地址，
		// 显式置 true 时才绑定全部网卡（跨机部署琉璃/月华时开启）
		AllowLAN *bool `json:"allow_lan,omitempty"`
		// 插件沙箱网络是否跳过 TLS 证书校验：未配置默认开启校验；置 true 恢复旧的跳过行为
		InsecureTLS *bool `json:"insecure_tls,omitempty"`
	} `json:"server"`
	// 核心智能体模型配置（月华 Agent）
	Agent struct {
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
	} `json:"agent"`
	// 记忆库独立模型配置（优先于旧版 server/models 全局配置）
	Memory struct {
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
	} `json:"memory"`
	// 智能搜索独立模型配置（优先于旧版 server/models 全局配置）
	Search struct {
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
	} `json:"search"`
}

// init 加载配置文件
func init() {
	// 测试二进制会注入 -test.* 标志（Go 1.24+ 如 -test.testlogfile），本包 init 阶段它们尚未
	// 注册（testing.Init 才注册），直接 flag.Parse() 会因标志未定义而失败。这里把它们从本次
	// 解析列表剔除，但保持 os.Args 原样——测试框架（testing.Main）随后仍能读取自己的 -test.*
	// 参数，否则 go test 的 -v/-run/-list 等会全部失效。
	filtered := make([]string, 0, len(os.Args))
	for _, arg := range os.Args {
		if !strings.HasPrefix(arg, "-test.") {
			filtered = append(filtered, arg)
		}
	}
	// 解析命令行参数（仅业务标志；无可解析参数时跳过）
	if len(filtered) > 1 {
		_ = flag.CommandLine.Parse(filtered[1:])
	}
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
	// 如果配置文件中 PromptAnalysisModel 字段非空，则更新全局配置
	if parameter.Models.PromptAnalysisModel != "" {
		*PromptAnalysisModel = parameter.Models.PromptAnalysisModel
	}
	// 如果配置文件中 RealESRGANModel 字段非空，则更新全局配置
	if parameter.Models.RealESRGANModel != "" {
		*RealESRGANModel = parameter.Models.RealESRGANModel
	}
	// 如果配置文件中 AsrModel 字段非空，则更新全局配置
	if parameter.Models.AsrModel != "" {
		*AsrModel = parameter.Models.AsrModel
	}

	// ==== 记忆库独立配置（memory，优先于旧版 server/models 全局配置） ====
	if parameter.Memory.EmbeddingModel != "" {
		*MemoryEmbeddingModel = parameter.Memory.EmbeddingModel
	}
	if parameter.Memory.EmbeddingURL != "" {
		*MemoryEmbeddingURL = parameter.Memory.EmbeddingURL
	}
	if parameter.Memory.EmbeddingKey != "" {
		*MemoryEmbeddingKey = parameter.Memory.EmbeddingKey
	}
	if parameter.Memory.MultimodalModel != "" {
		*MemoryMultimodalModel = parameter.Memory.MultimodalModel
	}
	if parameter.Memory.MultimodalURL != "" {
		*MemoryMultimodalURL = parameter.Memory.MultimodalURL
	}
	if parameter.Memory.MultimodalKey != "" {
		*MemoryMultimodalKey = parameter.Memory.MultimodalKey
	}

	// ==== 智能搜索独立配置（search，优先于旧版 server/models 全局配置） ====
	if parameter.Search.EmbeddingModel != "" {
		*SearchEmbeddingModel = parameter.Search.EmbeddingModel
	}
	if parameter.Search.EmbeddingURL != "" {
		*SearchEmbeddingURL = parameter.Search.EmbeddingURL
	}
	if parameter.Search.EmbeddingKey != "" {
		*SearchEmbeddingKey = parameter.Search.EmbeddingKey
	}
	if parameter.Search.MultimodalModel != "" {
		*SearchMultimodalModel = parameter.Search.MultimodalModel
	}
	if parameter.Search.MultimodalURL != "" {
		*SearchMultimodalURL = parameter.Search.MultimodalURL
	}
	if parameter.Search.MultimodalKey != "" {
		*SearchMultimodalKey = parameter.Search.MultimodalKey
	}
	// ==== 核心智能体配置（agent） ====
	if parameter.Agent.EmbeddingModel != "" {
		*AgentEmbeddingModel = parameter.Agent.EmbeddingModel
	}
	if parameter.Agent.EmbeddingURL != "" {
		*AgentEmbeddingURL = parameter.Agent.EmbeddingURL
	}
	if parameter.Agent.EmbeddingKey != "" {
		*AgentEmbeddingKey = parameter.Agent.EmbeddingKey
	}
	if parameter.Agent.MultimodalModel != "" {
		*AgentMultimodalModel = parameter.Agent.MultimodalModel
	}
	if parameter.Agent.MultimodalURL != "" {
		*AgentMultimodalURL = parameter.Agent.MultimodalURL
	}
	if parameter.Agent.MultimodalKey != "" {
		*AgentMultimodalKey = parameter.Agent.MultimodalKey
	}
	// 如果配置文件中 Developer 字段非空，则更新全局配置
	if parameter.Server.Developer == true {
		*Developer = true
	} else {
		*Developer = false
	}
	// 如果配置文件中 AllowDiffusion 字段非空，则更新全局配置
	if parameter.Server.AllowDiffusion == true {
		*AllowDiffusion = true
	} else {
		*AllowDiffusion = false
	}
	// 画图前显存守卫配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Server.ImageVRAMGuard != nil {
		*ImageVRAMGuard = *parameter.Server.ImageVRAMGuard
	}
	if parameter.Server.ImageVRAMGuardMiB != nil {
		*ImageVRAMGuardMiB = *parameter.Server.ImageVRAMGuardMiB
	}
	// sd.cpp 权重内存卸载与提示词编码器 CPU 运行配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Server.SDOffloadToCPU != nil && *parameter.Server.SDOffloadToCPU != "" {
		*SDOffloadToCPU = *parameter.Server.SDOffloadToCPU
	}
	if parameter.Server.SDTextEncoderOnCPU != nil {
		*SDTextEncoderOnCPU = *parameter.Server.SDTextEncoderOnCPU
	}
	// 引擎服务绑定范围与插件 TLS 校验：仅在配置文件中显式给出时才覆盖安全默认值
	if parameter.Server.AllowLAN != nil {
		*AllowLAN = *parameter.Server.AllowLAN
	}
	if parameter.Server.InsecureTLS != nil {
		*InsecureTLS = *parameter.Server.InsecureTLS
	}
}
