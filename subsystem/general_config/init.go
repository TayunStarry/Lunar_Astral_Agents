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
//
// 分组严格按职责边界划分：Models 只放模型文件路径、Server 只管服务运行，
// 其余每组对应且仅对应一个功能域，避免出现职责混杂的「杂项组」。
type ModelConfig struct {
	// 模型文件路径（辅助/生成类模型；主模型服务分段在 local_data/models/models.ini）
	Models struct {
		DiffusionModel      string `json:"diffusion_model"`       // 扩散模型路径
		VariationalModel    string `json:"variational_model"`     // 变分模型路径
		PromptAnalysisModel string `json:"prompt_analysis_model"` // 提示分析模型路径
		PromptMmprojModel   string `json:"prompt_mmproj_model"`   // 多模态提示分析模型路径
		RealESRGANModel     string `json:"real_esrgan_model"`     // 4x超分辨率模型路径
	} `json:"models"`
	// 服务运行与调试
	Server struct {
		Developer bool `json:"developer"` // 是否为开发者模式
		// 是否允许局域网访问引擎 HTTP/WS 服务：未配置默认仅绑定 127.0.0.1 回环地址，
		// 显式置 true 时才绑定全部网卡（跨机部署琉璃/月华时开启）
		AllowLAN *bool `json:"allow_lan,omitempty"`
		// 插件沙箱网络是否跳过 TLS 证书校验：未配置默认开启校验；置 true 恢复旧的跳过行为
		InsecureTLS *bool `json:"insecure_tls,omitempty"`
	} `json:"server"`
	// 月华 Agent：用户显示名 + 模型路由
	Agent struct {
		UserName        string `json:"user_name"`        // 用户显示名（前端气泡名与发送前缀）
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
	} `json:"agent"`
	// 记忆库模型路由（独立于 agent，可单独指向其它服务）
	Memory struct {
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
	} `json:"memory"`
	// 聊天（LLM）显存守卫：可用显存不足时卸载本地模型释放 KV 缓存。
	// 仅由月华智能体（TypeScript）读取，此处声明结构以集中呈现配置契约。
	Chat struct {
		ChatVRAMGuard         *bool `json:"chat_vram_guard,omitempty"`          // 守卫开关
		ChatVRAMGuardMiB      *int  `json:"chat_vram_guard_mib,omitempty"`      // 触发阈值（MiB）
		ChatVRAMGuardInterval *int  `json:"chat_vram_guard_interval,omitempty"` // 每 N 次应答检查一次
	} `json:"chat"`
	// 图像生成（sd.cpp）行为与资源策略
	Diffusion struct {
		AllowDiffusion bool `json:"allow_diffusion"` // 是否允许运行扩散生成
		// 画图前显存守卫：使用指针以便区分"未配置"与"显式关闭"，未配置时保持默认值（开启，阈值 8192 MiB）
		ImageVRAMGuard    *bool `json:"image_vram_guard,omitempty"`
		ImageVRAMGuardMiB *int  `json:"image_vram_guard_mib,omitempty"`
		// sd.cpp 权重内存卸载模式（auto/always/off）与提示词编码器 CPU 运行开关
		SDOffloadToCPU     *string `json:"sd_offload_to_cpu,omitempty"`
		SDTextEncoderOnCPU *bool   `json:"sd_te_on_cpu,omitempty"`
		// VAE 分片解码策略与几何（off/always/auto；tile 以 latent 单元计，overlap 为比例）
		SDVaeTiling      *string  `json:"sd_vae_tiling,omitempty"`
		SDVaeTileSize    *string  `json:"sd_vae_tile_size,omitempty"`
		SDVaeTileOverlap *float64 `json:"sd_vae_tile_overlap,omitempty"`
		// 显存预算（GiB，空=自动）与采样策略（auto=交给 sd.cpp 按模型选择）
		SDMaxVRAM       *string  `json:"sd_max_vram,omitempty"`
		SDSampler       *string  `json:"sd_sampler,omitempty"`
		SDScheduler     *string  `json:"sd_scheduler,omitempty"`
		SDFlowShift     *float64 `json:"sd_flow_shift,omitempty"`
		SDFlashAttention *bool    `json:"sd_diffusion_fa,omitempty"`
		// 调用方未指定时的默认步数与边长
		SDSteps     *int `json:"sd_steps,omitempty"`
		SDImageSize *int `json:"sd_image_size,omitempty"`
	} `json:"diffusion"`
	// NapCat QQ 桥接。仅由月华的 bridging/napcat 读取，此处声明结构以集中呈现配置契约。
	Bridging struct {
		BridgingType                    string   `json:"bridging_type"`                     // 桥接类型（napcat）
		BridgingPath                    string   `json:"bridging_path"`                     // 桥接地址（NapCat WS 地址）
		BridgingToken                   string   `json:"bridging_token"`                    // 桥接令牌
		BridgingUsers                   []int64  `json:"bridging_users"`                    // 允许响应的用户/群号列表
		BridgingGroupKeywords           []string `json:"bridging_group_keywords"`           // 群聊触发关键词
		BridgingGroupTriggerProbability *float64 `json:"bridging_group_trigger_probability"` // 群聊无关键词时的应答概率
	} `json:"bridging"`
	// 多模态解读配置（视频/动态图送入模型的方式）
	Multimodal struct {
		// 视频解读输入模式：file=写媒体目录后以 file:// 引用交 llama-server 抽帧（默认）；
		// frames=本地抽帧为图片序列帧（base64）后以多个 image_url 提交，供不支持 file:// 的架构使用
		VideoInput string `json:"video_input"`
	} `json:"multimodal"`
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
	// ==== 模型文件路径（models） ====
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
	// 如果配置文件中 PromptMmprojModel 字段非空，则更新全局配置
	if parameter.Models.PromptMmprojModel != "" {
		*PromptMmprojModel = parameter.Models.PromptMmprojModel
	}
	// 如果配置文件中 RealESRGANModel 字段非空，则更新全局配置
	if parameter.Models.RealESRGANModel != "" {
		*RealESRGANModel = parameter.Models.RealESRGANModel
	}

	// ==== 记忆库模型路由（memory） ====
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
	// ==== 多模态解读配置（multimodal） ====
	// 视频解读输入模式：仅接受 file / frames，未配置或取值非法时保持默认 file（历史行为）
	switch parameter.Multimodal.VideoInput {
	case "frames":
		*VideoInputMode = "frames"
	case "file":
		*VideoInputMode = "file"
	}
	// ==== 图像生成行为与资源策略（diffusion） ====
	// 如果配置文件中 AllowDiffusion 字段非空，则更新全局配置
	if parameter.Diffusion.AllowDiffusion == true {
		*AllowDiffusion = true
	} else {
		*AllowDiffusion = false
	}
	// 画图前显存守卫配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.ImageVRAMGuard != nil {
		*ImageVRAMGuard = *parameter.Diffusion.ImageVRAMGuard
	}
	if parameter.Diffusion.ImageVRAMGuardMiB != nil {
		*ImageVRAMGuardMiB = *parameter.Diffusion.ImageVRAMGuardMiB
	}
	// sd.cpp 权重内存卸载与提示词编码器 CPU 运行配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.SDOffloadToCPU != nil && *parameter.Diffusion.SDOffloadToCPU != "" {
		*SDOffloadToCPU = *parameter.Diffusion.SDOffloadToCPU
	}
	if parameter.Diffusion.SDTextEncoderOnCPU != nil {
		*SDTextEncoderOnCPU = *parameter.Diffusion.SDTextEncoderOnCPU
	}
	// VAE 分片解码策略：仅在配置文件显式给出非空值时才覆盖默认值（默认 off）
	if parameter.Diffusion.SDVaeTiling != nil && *parameter.Diffusion.SDVaeTiling != "" {
		*SDVaeTiling = *parameter.Diffusion.SDVaeTiling
	}
	if parameter.Diffusion.SDVaeTileSize != nil && *parameter.Diffusion.SDVaeTileSize != "" {
		*SDVaeTileSize = *parameter.Diffusion.SDVaeTileSize
	}
	if parameter.Diffusion.SDVaeTileOverlap != nil {
		*SDVaeTileOverlap = *parameter.Diffusion.SDVaeTileOverlap
	}
	// 显存预算与采样策略：空字符串表示"使用引擎自动决策"，因此只在非空时覆盖
	if parameter.Diffusion.SDMaxVRAM != nil {
		*SDMaxVRAM = *parameter.Diffusion.SDMaxVRAM
	}
	if parameter.Diffusion.SDSampler != nil && *parameter.Diffusion.SDSampler != "" {
		*SDSampler = *parameter.Diffusion.SDSampler
	}
	if parameter.Diffusion.SDScheduler != nil && *parameter.Diffusion.SDScheduler != "" {
		*SDScheduler = *parameter.Diffusion.SDScheduler
	}
	if parameter.Diffusion.SDFlowShift != nil {
		*SDFlowShift = *parameter.Diffusion.SDFlowShift
	}
	if parameter.Diffusion.SDFlashAttention != nil {
		*SDFlashAttention = *parameter.Diffusion.SDFlashAttention
	}
	// 缺省步数与边长：仅接受正数，避免非法配置把生成参数打成 0
	if parameter.Diffusion.SDSteps != nil && *parameter.Diffusion.SDSteps > 0 {
		*SDSteps = *parameter.Diffusion.SDSteps
	}
	if parameter.Diffusion.SDImageSize != nil && *parameter.Diffusion.SDImageSize > 0 {
		*SDImageSize = *parameter.Diffusion.SDImageSize
	}
	// ==== 服务运行与调试（server） ====
	// 如果配置文件中 Developer 字段非空，则更新全局配置
	if parameter.Server.Developer == true {
		*Developer = true
	} else {
		*Developer = false
	}
	// 引擎服务绑定范围与插件 TLS 校验：仅在配置文件中显式给出时才覆盖安全默认值
	if parameter.Server.AllowLAN != nil {
		*AllowLAN = *parameter.Server.AllowLAN
	}
	if parameter.Server.InsecureTLS != nil {
		*InsecureTLS = *parameter.Server.InsecureTLS
	}
	// chat（聊天显存守卫）与 bridging（QQ 桥接）不落 Go 全局：
	// 前者仅由月华 TypeScript 读取，后者由 lunar_astral/bridging/napcat 自行解析
}
