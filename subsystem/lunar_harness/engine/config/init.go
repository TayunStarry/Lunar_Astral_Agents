package config

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// ModelScheme 一套绘图模型方案的原始配置（与 DiffusionScheme 一一对应）
//
// 扩散模型、VAE、提示词编码模型必须成套匹配，因此按方案整组声明，
// 避免调用方把不同方案的模型混用成非法组合。
type ModelScheme struct {
	Label               string `json:"label"`                 // 界面显示名（空则用方案键名）
	DiffusionModel      string `json:"diffusion_model"`       // 扩散模型路径
	VariationalModel    string `json:"variational_model"`     // 变分模型路径
	PromptAnalysisModel string `json:"prompt_analysis_model"` // 提示分析模型路径
	PromptMmprojModel   string `json:"prompt_mmproj_model"`   // 多模态提示分析模型路径（可空）
	RealESRGANModel     string `json:"real_esrgan_model"`     // 4x超分辨率模型路径（可空）
	Steps               int    `json:"steps"`                 // 方案推荐默认步数（0=沿用 sd_steps）
}

// SystemSettings 系统级设置的统一根对象。
//
// 把原先分散的 server / logs / chat / client 四节扁平收拢到 system 下：
// 运行与安全、控制台日志、聊天显存守卫、前端显示共用同一个根对象。
type SystemSettings struct {
	Developer bool `json:"developer"` // 是否为开发者模式
	// 是否允许局域网访问引擎 HTTP/WS 服务：未配置默认仅绑定 127.0.0.1 回环地址，
	// 显式置 true 时才绑定全部网卡（跨机部署琉璃/月华时开启）
	AllowLAN *bool `json:"allow_lan,omitempty"`
	// 插件沙箱网络是否跳过 TLS 证书校验：未配置默认开启校验；置 true 恢复旧的跳过行为
	InsecureTLS *bool `json:"insecure_tls,omitempty"`
	// 是否把控制台输出镜像到 local_data/logs/console.log；未配置默认关闭
	ConsoleLog *bool `json:"console_log,omitempty"`
	// 聊天（LLM）显存守卫：可用显存不足时卸载本地模型释放 KV 缓存
	ChatVRAMGuard         *bool `json:"chat_vram_guard,omitempty"`          // 守卫开关
	ChatVRAMGuardMiB      *int  `json:"chat_vram_guard_mib,omitempty"`      // 触发阈值（MiB）
	ChatVRAMGuardInterval *int  `json:"chat_vram_guard_interval,omitempty"` // 每 N 次应答检查一次
	// 月华客户端显示：控制桌面端/移动端角色的渲染方式（bedrock 默认 / live2d）
	CharacterRenderer string `json:"character_renderer"`
	Live2DModel       string `json:"live2d_model,omitempty"` // Live2D 模型基址（空则用引擎层默认）
	// 模型重复输出检测（loopguard）：节点字数 / 片段字数 / 平均相似度阈值。
	// 未配置时沿用 loopguard 默认值（1024 / 128 / 0.65），便于按实际效果调参。
	LoopNodeRunes    *int     `json:"loop_node_runes,omitempty"`
	LoopSegmentRunes *int     `json:"loop_segment_runes,omitempty"`
	LoopSimilarity   *float64 `json:"loop_similarity,omitempty"`
}

// ModelConfig 定义模型配置的结构
//
// 分组严格按职责边界划分：Models 只放模型文件路径、System 放运行/日志/聊天守卫/客户端显示，
// 其余每组对应且仅对应一个功能域，避免出现职责混杂的「杂项组」。
type ModelConfig struct {
	// 模型文件路径（辅助/生成类模型；主模型服务分段在 local_data/models/models.ini）
	Models struct {
		// 可切换的绘图模型方案：键名（turbo / quality）→ 成套模型路径。
		// 未配置时回退到下面的扁平模型路径，保持旧配置文件可用。
		Schemes             map[string]ModelScheme `json:"schemes"`
		DefaultScheme       string                 `json:"default_scheme"`        // 默认方案键名（缺省取排序后的第一个）
		DiffusionModel      string                 `json:"diffusion_model"`       // 扩散模型路径
		VariationalModel    string                 `json:"variational_model"`     // 变分模型路径
		PromptAnalysisModel string                 `json:"prompt_analysis_model"` // 提示分析模型路径
		PromptMmprojModel   string                 `json:"prompt_mmproj_model"`   // 多模态提示分析模型路径
		RealESRGANModel     string                 `json:"real_esrgan_model"`     // 4x超分辨率模型路径
	} `json:"models"`
	// 系统级设置：运行/安全、日志、聊天显存守卫、前端显示共用同一个根对象
	System SystemSettings `json:"system"`
	// 月华 Agent：用户显示名 + 模型路由 + 多模态输入方式
	Agent struct {
		UserName        string `json:"user_name"`        // 用户显示名（前端气泡名与发送前缀）
		EmbeddingModel  string `json:"embedding_model"`  // 嵌入模型名称
		EmbeddingURL    string `json:"embedding_url"`    // 嵌入服务 API 地址
		EmbeddingKey    string `json:"embedding_key"`    // 嵌入服务 API 密钥
		MultimodalModel string `json:"multimodal_model"` // 多模态模型名称
		MultimodalURL   string `json:"multimodal_url"`   // 多模态服务 API 地址
		MultimodalKey   string `json:"multimodal_key"`   // 多模态服务 API 密钥
		// 语音识别模型名称（ASR）
		ASRModel string `json:"asr_model"`
		// 视频解读输入模式：file=写媒体目录后以 file:// 引用交 llama-server 抽帧（默认）；
		// frames=本地抽帧为图片序列帧（base64）后以多个 image_url 提交，供不支持 file:// 的架构使用
		VideoInput string `json:"video_input"`
		// 静态图思维链形态：url=视觉读取端点 URL（默认）/ base64=变换管线直出 data URI（云端服务用）
		ImageInput string `json:"image_input"`
		// 视觉读取副本最大边长（像素），默认 1080
		VisionMaxSide int `json:"vision_max_side"`
		// 视觉副本缓存池容量上限（MB），超限按最久未用淘汰，默认 512
		VisionCacheCapMB int `json:"vision_cache_cap_mb"`
		// 引擎自愈：应答连续失败达阈值时自动重建月华对话链状态
		// 指针语义：未配置时保持默认（开启、阈值 2 次）
		AutoReload         *bool `json:"auto_reload,omitempty"`
		AutoReloadFailures *int  `json:"auto_reload_failures,omitempty"`
		// 追加设定：逐行附加到「月华 · 对话者」系统提示词末尾（由 Go 对话链读取；
		// 未配置或空数组时不追加任何内容）
		AppendSettings []string `json:"append_settings"`
	} `json:"agent"`
	// 网络搜索智能体（web_search）：检索词提炼、逐页摘要与浏览器滚动策略。
	// 仅由琉璃的内置 Web-LTP（agents/WebSearch）消费，与其余配置节统一在此解析，
	// 智能体侧只读取解析后的全局值，不再自行读取 lunar_config.json。
	WebSearch struct {
		SearchModel     string `json:"search_model"`            // 检索词提炼与逐页摘要使用的模型名
		SwapModel       *bool  `json:"swap_model,omitempty"`    // 搜索前卸载主模型、加载搜索模型（未配置默认开启）
		MaxPages        int    `json:"max_pages"`               // 进入详情页的页数上限（智能体侧再施加硬上限）
		MaxResults      int    `json:"max_results"`             // 结果页解析的检索结果条数
		SummaryMaxChars int    `json:"summary_max_chars"`       // 单页摘要硬切断字符数
		MaxRunSeconds   int    `json:"max_run_seconds"`         // 单次检索总时长软上限（秒）
		CacheEnabled    *bool  `json:"cache_enabled,omitempty"` // 近期访问标记开关（未配置默认开启）
		CacheTTLDays    int    `json:"cache_ttl_days"`          // 近期访问标记的有效期（天）
		TextChunkChars  int    `json:"text_chunk_chars"`        // 文本摘要的单段字符数
		TextMaxChars    int    `json:"text_max_chars"`          // 单页正文提取硬上限
		TextMaxChunks   int    `json:"text_max_chunks"`         // 单页最多参与摘要的文本段数
		ScrollSettleMS  int    `json:"scroll_settle_ms"`        // 每次滚动后的等待毫秒数
		ScrollMaxSteps  int    `json:"scroll_max_steps"`        // 单页滚动步数上限
	} `json:"web_search"`
	// 记忆库不再有独立的模型路由：嵌入与多模态模型统一复用 agent 分组
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
		// VAE 解码后端（auto=放不进显存就整段改 CPU / gpu / cpu）与 GPU direct conv 开关
		SDVaeBackend    *string `json:"sd_vae_backend,omitempty"`
		SDVaeConvDirect *bool   `json:"sd_vae_conv_direct,omitempty"`
		// 显存预算（GiB，空=自动）与采样策略（auto=交给 sd.cpp 按模型选择）
		SDMaxVRAM        *string  `json:"sd_max_vram,omitempty"`
		SDSampler        *string  `json:"sd_sampler,omitempty"`
		SDScheduler      *string  `json:"sd_scheduler,omitempty"`
		SDFlowShift      *float64 `json:"sd_flow_shift,omitempty"`
		SDFlashAttention *bool    `json:"sd_diffusion_fa,omitempty"`
		// 调用方未指定时的默认步数与边长
		SDSteps     *int `json:"sd_steps,omitempty"`
		SDImageSize *int `json:"sd_image_size,omitempty"`
		// 图生图/合照按参考图比例推导尺寸时的总像素预算上限（百万像素，默认 1.6）
		PixelBudgetMP *float64 `json:"pixel_budget_mp,omitempty"`
		// 图生图的提示词引导系数覆盖值（<=0 表示不覆盖，按 1.0）
		Img2ImgCFG *float64 `json:"img2img_cfg,omitempty"`
	} `json:"diffusion"`
	// NapCat QQ 桥接。仅由月华的 bridging/napcat 读取，此处声明结构以集中呈现配置契约。
	Bridging struct {
		BridgingType                    string   `json:"bridging_type"`                      // 桥接类型（napcat）
		BridgingPath                    string   `json:"bridging_path"`                      // 桥接地址（NapCat WS 地址）
		BridgingToken                   string   `json:"bridging_token"`                     // 桥接令牌
		BridgingUsers                   []int64  `json:"bridging_users"`                     // 允许响应的用户/群号列表
		BridgingGroupKeywords           []string `json:"bridging_group_keywords"`            // 群聊触发关键词
		BridgingGroupTriggerProbability *float64 `json:"bridging_group_trigger_probability"` // 群聊无关键词时的应答概率
	} `json:"bridging"`
	// 多模态解读配置（视频/动态图送入模型的方式）：已并入 agent.video_input，
	// 此处仅保留旧节结构用于迁移兼容读取
	LegacyMultimodal struct {
		VideoInput string `json:"video_input"`
	} `json:"multimodal"`
	// 月华语音合成（TTS）：引擎选择 + 统一效果参数。
	// 参数层面统一：各引擎共用同一套字段；执行层面由 engine/broadcaster 统一播报
	// 链路按引擎自身格式适配，引擎不支持的参数（如 Kokoro 不支持参考音频）自动忽略。
	TTS struct {
		Engine          string        `json:"engine"`                     // 引擎枚举：Qwen / Kokoro / OmniVoice（默认 Qwen）
		RefAudio        string        `json:"ref_audio"`                  // 参考音频路径（Qwen/OmniVoice 声音克隆；Kokoro 忽略）
		RefText         string        `json:"ref_text"`                   // 参考音频转写文本（OmniVoice 声音克隆必填；其余忽略）
		Voice           string        `json:"voice"`                      // 音色名称（Kokoro；其余忽略）
		Mix             []TTSMixEntry `json:"mix"`                        // 音色混合列表（Kokoro；其余忽略）
		Speed           *float64      `json:"speed,omitempty"`            // 语速倍率（Kokoro 0.5~2.0；其余忽略）
		Lang            string        `json:"lang"`                       // 语言提示（Kokoro zh/en/auto；OmniVoice 语言提示；Qwen 忽略）
		Instruct        string        `json:"instruct"`                   // 音色属性关键词（OmniVoice；其余忽略）
		Seed            *uint64       `json:"seed,omitempty"`             // 随机种子（OmniVoice；其余忽略）
		DisablePostproc *bool         `json:"disable_postproc,omitempty"` // 关闭输出后处理（OmniVoice；其余忽略）
		// 引擎闲置卸载阈值（分钟）：无合成调用超过该时长即卸载模型释放占用
		IdleUnloadMinutes *int `json:"idle_unload_minutes,omitempty"`
	} `json:"tts"`
	// Gitee Release 发布通道（release）：更新包与 LTPX 应用市场共用同一仓库，
	// 仓库地址、令牌、打包清单、备份策略统一在此配置。
	// 两条通道在仓库内以「描述标记 + 附件名前缀」严格区分：
	// 更新包 body 含 [UPDATE] / 附件名 lunar_updatepack_*；市场包 body 含 [LTPX] / 附件名 ltpx_*.ltpx。
	// 指针类型用于区分「未配置 release 节」与「配置了但字段为空」——前者回退读取旧节。
	Release *struct {
		GiteeOwner string `json:"gitee_owner"` // Gitee 仓库属主
		GiteeRepo  string `json:"gitee_repo"`  // Gitee 仓库名
		GiteeToken string `json:"gitee_token"` // Gitee 私人令牌（需 Release 读写权限；留空则发布时在 TUI 手动输入）
		// ==== 更新包（资源 + 主程序） ====
		Include []string `json:"include"` // 打包包含的资源目录（相对 target_dir）
		// 打包排除规则（空时用默认清单）与「含这些文件的目录整体排除」（空时默认 metadata.json）
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		// 覆盖前备份的保留次数（nil=默认 3；0 或负数=不备份）
		BackupKeep *int `json:"backup_keep,omitempty"`
		// 主程序文件清单（相对程序根目录）：发布时纳入包，拉取时判定是否走「关程序→应用器→重启」
		ProgramFiles []string `json:"program_files"`
		// 重启主程序时携带的命令行参数
		RestartArgs []string `json:"restart_args"`
		// 资源根目录/程序根目录（空时默认 local_data；程序合并后取「程序根目录」）
		TargetDir string `json:"target_dir"`
		// ==== LTPX 应用市场 ====
		// 本地 LTPX 包根目录（空时默认 local_data/package）
		PackageDir string `json:"package_dir"`
	} `json:"release"`

	// ==== 旧配置节（迁移兼容，仅用于读取并提示迁移） ====
	// 保留读取能力：老版本 lunar_config.json 的 update_pack / ltpx_market / tui / multimodal
	// 仍可被识别，避免升级后配置静默失效（详见 init 中的回退与告警）。
	LegacyUpdatePack *struct {
		GiteeOwner      string   `json:"gitee_owner"`
		GiteeRepo       string   `json:"gitee_repo"`
		GiteeToken      string   `json:"gitee_token"`
		Include         []string `json:"include"`
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		BackupKeep      *int     `json:"backup_keep"`
		ProgramFiles    []string `json:"program_files"`
		RestartArgs     []string `json:"restart_args"`
		TargetDir       string   `json:"target_dir"`
	} `json:"update_pack"`
	LegacyLtpxMarket *struct {
		GiteeOwner string `json:"gitee_owner"`
		GiteeRepo  string `json:"gitee_repo"`
		PackageDir string `json:"package_dir"`
	} `json:"ltpx_market"`
	// LegacyTUI 旧 tui 节（检测用）：其唯一字段 exit_when_frontends_closed 已随桌面版取消
	// ——程序只有控制台一种形态，前端窗口全部关闭不再退出进程。
	// 保留结构仅为在配置仍含该节时给出明确提示，不作任何生效处理。
	LegacyTUI struct {
		ExitWhenFrontendsClosed *bool `json:"exit_when_frontends_closed"`
	} `json:"tui"`
}

// init 加载配置文件
func init() {
	// 先把全部配置项注册为命令行参数（必须在 flag.Parse 之前）
	RegisterFlags()
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
	configPath := filepath.Join(exeDir, LocalDir, "lunar_config.json")
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
	// 解析结果 → 全局配置（映射与旧节回退规则集中在 applyConfig，便于单测覆盖）
	legacyUsed := applyConfig(parameter)

	// ==== 旧配置节迁移提示（合并为一条日志，避免刷屏） ====
	if len(legacyUsed) > 0 {
		log.Printf("[Config][WARN] -> lunar_config.json 使用了旧配置节，已按兼容方式读取；建议迁移：%s",
			strings.Join(legacyUsed, "；"))
	}
}

// applyConfig 把解析结果写入全局配置，返回本次用到的旧配置节说明（迁移提示用）。
//
// 独立成函数：配置文件读的是 exe 同级的 lunar_config.json，init 的文件路径无法在测试中
// 复现，而「新节优先 / 旧节回退」这类规则必须被单测钉住——否则改动一处就会静默失效
// （用户升级后配置回到默认值，且没有任何报错）。
func applyConfig(parameter *ModelConfig) []string {
	// legacyUsed 记录本次读取用到的旧配置节
	var legacyUsed []string
	// ==== 模型文件路径（models） ====
	// 如果配置文件中 DiffusionModel 字段非空，则更新全局配置
	if parameter.Models.DiffusionModel != "" {
		DiffusionModel = parameter.Models.DiffusionModel
	}
	// 如果配置文件中 VariationalModel 字段非空，则更新全局配置
	if parameter.Models.VariationalModel != "" {
		VariationalModel = parameter.Models.VariationalModel
	}
	// 如果配置文件中 PromptAnalysisModel 字段非空，则更新全局配置
	if parameter.Models.PromptAnalysisModel != "" {
		PromptAnalysisModel = parameter.Models.PromptAnalysisModel
	}
	// 如果配置文件中 PromptMmprojModel 字段非空，则更新全局配置
	if parameter.Models.PromptMmprojModel != "" {
		PromptMmprojModel = parameter.Models.PromptMmprojModel
	}
	// 如果配置文件中 RealESRGANModel 字段非空，则更新全局配置
	if parameter.Models.RealESRGANModel != "" {
		RealESRGANModel = parameter.Models.RealESRGANModel
	}
	// 绘图模型方案（models.schemes）：注册后立即启用默认方案。
	// 未配置方案时保持上面的扁平模型路径，行为与旧配置一致。
	if len(parameter.Models.Schemes) > 0 {
		schemes := make(map[string]DiffusionScheme, len(parameter.Models.Schemes))
		for name, raw := range parameter.Models.Schemes {
			schemes[name] = DiffusionScheme{
				Label:               raw.Label,
				DiffusionModel:      raw.DiffusionModel,
				VariationalModel:    raw.VariationalModel,
				PromptAnalysisModel: raw.PromptAnalysisModel,
				PromptMmprojModel:   raw.PromptMmprojModel,
				RealESRGANModel:     raw.RealESRGANModel,
				Steps:               raw.Steps,
			}
		}
		SetDiffusionSchemes(parameter.Models.DefaultScheme, schemes)
	}

	// ==== 记忆库模型路由：已移除（统一复用 agent 分组的嵌入/多模态模型） ====
	// 旧版 lunar_config.json 的 memory 节会被 JSON 解码忽略；迁移提示由文档说明。
	// ==== 核心智能体配置（agent） ====
	if parameter.Agent.EmbeddingModel != "" {
		AgentEmbeddingModel = parameter.Agent.EmbeddingModel
	}
	if parameter.Agent.EmbeddingURL != "" {
		AgentEmbeddingURL = parameter.Agent.EmbeddingURL
	}
	if parameter.Agent.EmbeddingKey != "" {
		AgentEmbeddingKey = parameter.Agent.EmbeddingKey
	}
	if parameter.Agent.MultimodalModel != "" {
		AgentMultimodalModel = parameter.Agent.MultimodalModel
	}
	if parameter.Agent.MultimodalURL != "" {
		AgentMultimodalURL = parameter.Agent.MultimodalURL
	}
	if parameter.Agent.MultimodalKey != "" {
		AgentMultimodalKey = parameter.Agent.MultimodalKey
	}
	if parameter.Agent.ASRModel != "" {
		AgentASRModel = parameter.Agent.ASRModel
	}
	// Go 原生对话链直接消费的用户显示名与追加设定
	AgentUserName = parameter.Agent.UserName
	AgentAppendSettings = append([]string(nil), parameter.Agent.AppendSettings...)
	// 绘图尺寸预算与图生图 CFG 覆盖
	if parameter.Diffusion.PixelBudgetMP != nil && *parameter.Diffusion.PixelBudgetMP > 0 {
		DiffusionPixelBudgetMP = *parameter.Diffusion.PixelBudgetMP
	}
	if parameter.Diffusion.Img2ImgCFG != nil && *parameter.Diffusion.Img2ImgCFG > 0 {
		DiffusionImg2ImgCFG = *parameter.Diffusion.Img2ImgCFG
	}
	// ==== 多模态解读配置（agent.video_input；兼容旧 multimodal 节） ====
	// 视频解读输入模式：仅接受 file / frames，未配置或取值非法时保持默认 file（历史行为）
	videoInput := parameter.Agent.VideoInput
	if videoInput == "" && parameter.LegacyMultimodal.VideoInput != "" {
		videoInput = parameter.LegacyMultimodal.VideoInput
		legacyUsed = append(legacyUsed, "multimodal.video_input → agent.video_input")
	}
	switch videoInput {
	case "frames":
		VideoInputMode = "frames"
	case "file":
		VideoInputMode = "file"
	}
	// ==== 静态图思维链形态（agent.image_input / agent.vision_max_side） ====
	// url=视觉读取端点（默认）；base64=视觉读取管线直出的副本（供无法回取本机 URL 的云端模型）。
	// 两种形态都是 ≤ vision_max_side 的副本，原图永远不进入思维链（见 docs/adr/0008）。
	switch parameter.Agent.ImageInput {
	case "url":
		ImageInputMode = "url"
	case "base64":
		ImageInputMode = "base64"
	case "":
		// 未配置时保持默认
	default:
		log.Printf("[Config][WARN] -> agent.image_input 取值非法: %q（允许 url/base64），保持默认 %s", parameter.Agent.ImageInput, ImageInputMode)
	}
	if parameter.Agent.VisionMaxSide > 0 {
		VisionMaxSide = parameter.Agent.VisionMaxSide
	}
	if parameter.Agent.VisionCacheCapMB > 0 {
		VisionCacheCapMB = parameter.Agent.VisionCacheCapMB
	}
	// ==== 对话链自愈（自动重建月华对话链状态）====
	// 仅在配置显式给出时覆盖默认值（开启 / 连续失败 2 次）
	if parameter.Agent.AutoReload != nil {
		EngineAutoReload = *parameter.Agent.AutoReload
	}
	if parameter.Agent.AutoReloadFailures != nil && *parameter.Agent.AutoReloadFailures > 0 {
		EngineAutoReloadFailures = *parameter.Agent.AutoReloadFailures
	}
	// ==== 月华语音合成（tts） ====
	// 引擎枚举：仅接受 Qwen / Kokoro / OmniVoice，未配置保持默认 Qwen，非法取值告警并保留默认
	switch parameter.TTS.Engine {
	case "Qwen", "Kokoro", "OmniVoice":
		TTSEngine = parameter.TTS.Engine
	case "":
		// 未配置时保持默认引擎
	default:
		log.Printf("[Config][WARN] -> tts.engine 取值非法: %q（允许 Qwen/Kokoro/OmniVoice），保持默认 %s", parameter.TTS.Engine, TTSEngine)
	}
	if parameter.TTS.RefAudio != "" {
		TTSRefAudio = parameter.TTS.RefAudio
	}
	if parameter.TTS.RefText != "" {
		TTSRefText = parameter.TTS.RefText
	}
	if parameter.TTS.Voice != "" {
		TTSVoice = parameter.TTS.Voice
	}
	// 音色混合列表：数组无法经命令行覆盖，仅由配置文件填充
	if parameter.TTS.Mix != nil {
		TTSMix = parameter.TTS.Mix
	}
	if parameter.TTS.Speed != nil {
		TTSSpeed = *parameter.TTS.Speed
	}
	if parameter.TTS.Lang != "" {
		TTSLang = parameter.TTS.Lang
	}
	if parameter.TTS.Instruct != "" {
		TTSInstruct = parameter.TTS.Instruct
	}
	if parameter.TTS.Seed != nil {
		TTSSeed = *parameter.TTS.Seed
	}
	if parameter.TTS.DisablePostproc != nil {
		TTSDisablePostproc = *parameter.TTS.DisablePostproc
	}
	// 闲置卸载阈值：仅接受正数分钟，非法取值（<=0）保留默认 5 分钟
	if parameter.TTS.IdleUnloadMinutes != nil && *parameter.TTS.IdleUnloadMinutes > 0 {
		TTSIdleUnloadMinutes = *parameter.TTS.IdleUnloadMinutes
	}
	// ==== 月华客户端显示配置（system.character_renderer） ====
	// 渲染器枚举：仅接受 bedrock / live2d，未配置保持默认 bedrock，非法取值告警并保留默认
	switch parameter.System.CharacterRenderer {
	case "bedrock", "live2d":
		CharacterRenderer = parameter.System.CharacterRenderer
	case "":
		// 未配置时保持默认渲染器
	default:
		log.Printf("[Config][WARN] -> system.character_renderer 取值非法: %q（允许 bedrock/live2d），保持默认 %s", parameter.System.CharacterRenderer, CharacterRenderer)
	}
	if parameter.System.Live2DModel != "" {
		Live2DModel = parameter.System.Live2DModel
	}
	// ==== Gitee Release 发布通道（release） ====
	// 更新包与 LTPX 应用市场共用同一仓库：两条通道在仓库内以「描述标记 + 附件名前缀」
	// 区分（[UPDATE]/lunar_updatepack_* 与 [LTPX]/ltpx_*.ltpx），因此配置只需一套仓库信息。
	// 未配置 release 节时回退读取旧的 update_pack / ltpx_market 两节并提示迁移。
	switch {
	case parameter.Release != nil:
		r := parameter.Release
		ReleaseGiteeOwner = r.GiteeOwner
		ReleaseGiteeRepo = r.GiteeRepo
		ReleaseGiteeToken = r.GiteeToken
		if r.Include != nil {
			ReleaseInclude = r.Include
		}
		if r.Exclude != nil {
			ReleaseExclude = r.Exclude
		}
		if r.ExcludeDirsWith != nil {
			ReleaseExcludeDirsWith = r.ExcludeDirsWith
		}
		if r.BackupKeep != nil {
			ReleaseBackupKeep = r.BackupKeep
		}
		if r.ProgramFiles != nil {
			ReleaseProgramFiles = r.ProgramFiles
		}
		if r.RestartArgs != nil {
			ReleaseRestartArgs = r.RestartArgs
		}
		ReleaseTargetDir = r.TargetDir
		ReleasePackageDir = r.PackageDir
		if parameter.LegacyUpdatePack != nil || parameter.LegacyLtpxMarket != nil {
			log.Printf("[Config][WARN] -> 检测到旧配置节 update_pack/ltpx_market，已被 release 节取代，旧节不再生效，请删除")
		}

	case parameter.LegacyUpdatePack != nil || parameter.LegacyLtpxMarket != nil:
		legacyUsed = append(legacyUsed, "update_pack / ltpx_market → release")
		if l := parameter.LegacyUpdatePack; l != nil {
			ReleaseGiteeOwner = l.GiteeOwner
			ReleaseGiteeRepo = l.GiteeRepo
			ReleaseGiteeToken = l.GiteeToken
			ReleaseInclude = l.Include
			ReleaseExclude = l.Exclude
			ReleaseExcludeDirsWith = l.ExcludeDirsWith
			ReleaseBackupKeep = l.BackupKeep
			ReleaseProgramFiles = l.ProgramFiles
			ReleaseRestartArgs = l.RestartArgs
			ReleaseTargetDir = l.TargetDir
		}
		if l := parameter.LegacyLtpxMarket; l != nil {
			// 旧市场的仓库字段留空时回退更新包仓库（历史行为：默认同仓库）
			if strings.TrimSpace(ReleaseGiteeOwner) == "" {
				ReleaseGiteeOwner = l.GiteeOwner
			}
			if strings.TrimSpace(ReleaseGiteeRepo) == "" {
				ReleaseGiteeRepo = l.GiteeRepo
			}
			ReleasePackageDir = l.PackageDir
		}
	}
	// ==== 网络搜索智能体（web_search） ====
	// 未配置或非法（<=0）的数值字段沿用默认值；布尔字段仅在显式给出时覆盖。
	if parameter.WebSearch.SearchModel != "" {
		WebSearchModel = parameter.WebSearch.SearchModel
	}
	if v := parameter.WebSearch.SwapModel; v != nil {
		b := *v
		WebSearchSwapModel = &b
	}
	if v := parameter.WebSearch.CacheEnabled; v != nil {
		b := *v
		WebSearchCacheEnabled = &b
	}
	if parameter.WebSearch.MaxPages > 0 {
		WebSearchMaxPages = parameter.WebSearch.MaxPages
	}
	if parameter.WebSearch.MaxResults > 0 {
		WebSearchMaxResults = parameter.WebSearch.MaxResults
	}
	if parameter.WebSearch.SummaryMaxChars > 0 {
		WebSearchSummaryMaxChars = parameter.WebSearch.SummaryMaxChars
	}
	if parameter.WebSearch.MaxRunSeconds > 0 {
		WebSearchMaxRunSeconds = parameter.WebSearch.MaxRunSeconds
	}
	if parameter.WebSearch.CacheTTLDays > 0 {
		WebSearchCacheTTLDays = parameter.WebSearch.CacheTTLDays
	}
	if parameter.WebSearch.TextChunkChars > 0 {
		WebSearchTextChunkChars = parameter.WebSearch.TextChunkChars
	}
	if parameter.WebSearch.TextMaxChars > 0 {
		WebSearchTextMaxChars = parameter.WebSearch.TextMaxChars
	}
	if parameter.WebSearch.TextMaxChunks > 0 {
		WebSearchTextMaxChunks = parameter.WebSearch.TextMaxChunks
	}
	if parameter.WebSearch.ScrollSettleMS > 0 {
		WebSearchScrollSettleMS = parameter.WebSearch.ScrollSettleMS
	}
	if parameter.WebSearch.ScrollMaxSteps > 0 {
		WebSearchScrollMaxSteps = parameter.WebSearch.ScrollMaxSteps
	}
	// ==== 图像生成行为与资源策略（diffusion） ====
	// 如果配置文件中 AllowDiffusion 字段非空，则更新全局配置
	if parameter.Diffusion.AllowDiffusion == true {
		AllowDiffusion = true
	} else {
		AllowDiffusion = false
	}
	// 画图前显存守卫配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.ImageVRAMGuard != nil {
		ImageVRAMGuard = *parameter.Diffusion.ImageVRAMGuard
	}
	if parameter.Diffusion.ImageVRAMGuardMiB != nil {
		ImageVRAMGuardMiB = *parameter.Diffusion.ImageVRAMGuardMiB
	}
	// sd.cpp 权重内存卸载与提示词编码器 CPU 运行配置：仅在配置文件中显式给出时才覆盖默认值
	if parameter.Diffusion.SDOffloadToCPU != nil && *parameter.Diffusion.SDOffloadToCPU != "" {
		SDOffloadToCPU = *parameter.Diffusion.SDOffloadToCPU
	}
	if parameter.Diffusion.SDTextEncoderOnCPU != nil {
		SDTextEncoderOnCPU = *parameter.Diffusion.SDTextEncoderOnCPU
	}
	// VAE 分片解码策略：仅在配置文件显式给出非空值时才覆盖默认值（默认 off）
	if parameter.Diffusion.SDVaeTiling != nil && *parameter.Diffusion.SDVaeTiling != "" {
		SDVaeTiling = *parameter.Diffusion.SDVaeTiling
	}
	if parameter.Diffusion.SDVaeTileSize != nil && *parameter.Diffusion.SDVaeTileSize != "" {
		SDVaeTileSize = *parameter.Diffusion.SDVaeTileSize
	}
	if parameter.Diffusion.SDVaeTileOverlap != nil {
		SDVaeTileOverlap = *parameter.Diffusion.SDVaeTileOverlap
	}
	// VAE 解码后端与 direct conv：仅显式给出时覆盖（默认 auto / 关闭）
	if parameter.Diffusion.SDVaeBackend != nil && *parameter.Diffusion.SDVaeBackend != "" {
		SDVaeBackend = *parameter.Diffusion.SDVaeBackend
	}
	if parameter.Diffusion.SDVaeConvDirect != nil {
		SDVaeConvDirect = *parameter.Diffusion.SDVaeConvDirect
	}
	// 显存预算与采样策略：空字符串表示"使用引擎自动决策"，因此只在非空时覆盖
	if parameter.Diffusion.SDMaxVRAM != nil {
		SDMaxVRAM = *parameter.Diffusion.SDMaxVRAM
	}
	if parameter.Diffusion.SDSampler != nil && *parameter.Diffusion.SDSampler != "" {
		SDSampler = *parameter.Diffusion.SDSampler
	}
	if parameter.Diffusion.SDScheduler != nil && *parameter.Diffusion.SDScheduler != "" {
		SDScheduler = *parameter.Diffusion.SDScheduler
	}
	if parameter.Diffusion.SDFlowShift != nil {
		SDFlowShift = *parameter.Diffusion.SDFlowShift
	}
	if parameter.Diffusion.SDFlashAttention != nil {
		SDFlashAttention = *parameter.Diffusion.SDFlashAttention
	}
	// 缺省步数与边长：仅接受正数，避免非法配置把生成参数打成 0
	if parameter.Diffusion.SDSteps != nil && *parameter.Diffusion.SDSteps > 0 {
		SDSteps = *parameter.Diffusion.SDSteps
	}
	if parameter.Diffusion.SDImageSize != nil && *parameter.Diffusion.SDImageSize > 0 {
		SDImageSize = *parameter.Diffusion.SDImageSize
	}
	// ==== 服务运行与调试（system） ====
	// 如果配置文件中 Developer 字段非空，则更新全局配置
	if parameter.System.Developer == true {
		Developer = true
	} else {
		Developer = false
	}
	// 引擎服务绑定范围与插件 TLS 校验：仅在配置文件中显式给出时才覆盖安全默认值
	if parameter.System.AllowLAN != nil {
		AllowLAN = *parameter.System.AllowLAN
	}
	if parameter.System.InsecureTLS != nil {
		InsecureTLS = *parameter.System.InsecureTLS
	}
	// ==== 控制台日志（system.console_log） ====
	// 控制台日志镜像：仅在配置中显式开启时落盘，未配置保持默认关闭
	if parameter.System.ConsoleLog != nil {
		ConsoleLogEnabled = *parameter.System.ConsoleLog
	}
	// ==== 模型重复输出检测（system.loop_*） ====
	// 只接受合法取值，非法项保持默认（由 loopguard 包兜底），避免把检测关死。
	if parameter.System.LoopNodeRunes != nil && *parameter.System.LoopNodeRunes > 0 {
		LoopNodeRunes = *parameter.System.LoopNodeRunes
	}
	if parameter.System.LoopSegmentRunes != nil && *parameter.System.LoopSegmentRunes > 0 {
		LoopSegmentRunes = *parameter.System.LoopSegmentRunes
	}
	if parameter.System.LoopSimilarity != nil && *parameter.System.LoopSimilarity > 0 && *parameter.System.LoopSimilarity < 1 {
		LoopSimilarity = *parameter.System.LoopSimilarity
	}
	// ==== 旧 tui 节（已取消） ====
	// 程序只剩控制台一种形态：前端窗口全部关闭不再退出进程，故该节已无对应功能，
	// 仅提示用户删除，避免「配置写了却没生效」的无解困惑
	if parameter.LegacyTUI.ExitWhenFrontendsClosed != nil {
		legacyUsed = append(legacyUsed, "tui.exit_when_frontends_closed 已取消（控制台常驻，前端窗口关闭不再退出程序）")
	}

	// ==== 旧配置节迁移提示由调用方（init）输出 ====
	// chat（system.chat_vram_guard 聊天显存守卫）与 bridging（QQ 桥接）不落 Go 全局：
	// 前者仅由月华 TypeScript 读取，后者由 lunar_astral/bridging/napcat 自行解析
	return legacyUsed
}
