package config

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

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
	// 启动时是否自动用默认参数启动 HTTPS 桥接（自签证书反向代理，供手机等外部设备接入）。
	// 未配置默认关闭；开启后后端端口取 BasicPort、代理端口取 ProxyPort、绑定首个本机 IP。
	AutoHTTPSProxy *bool `json:"auto_https_proxy,omitempty"`
	// 聊天（LLM）显存守卫：可用显存不足时卸载本地模型释放 KV 缓存
	ChatVRAMGuard         *bool `json:"chat_vram_guard,omitempty"`          // 守卫开关
	ChatVRAMGuardMiB      *int  `json:"chat_vram_guard_mib,omitempty"`      // 触发阈值（MiB）
	ChatVRAMGuardInterval *int  `json:"chat_vram_guard_interval,omitempty"` // 每 N 次应答检查一次
	// 月华客户端显示：控制桌面端/移动端角色的渲染方式（bedrock 默认 / live2d / gif）
	CharacterRenderer string `json:"character_renderer"`
	Live2DModel       string `json:"live2d_model,omitempty"` // Live2D 模型基址（空则用引擎层默认）
	// 模型重复输出检测（loopguard）：节点字数 / 片段字数 / 平均相似度阈值。
	// 未配置时沿用 loopguard 默认值（1024 / 128 / 0.65），便于按实际效果调参。
	LoopNodeRunes    *int     `json:"loop_node_runes,omitempty"`
	LoopSegmentRunes *int     `json:"loop_segment_runes,omitempty"`
	LoopSimilarity   *float64 `json:"loop_similarity,omitempty"`
}

// ModelConfig 定义公共配置（general.json）的结构
//
// 绘图相关配置（模型方案 + 扩散生成策略）已拆到独立文件 local_data/config/painter.json，
// 这里只保留与绘图无关的配置节；每组对应且仅对应一个功能域，避免出现职责混杂的「杂项组」。
type ModelConfig struct {
	// 系统级设置：运行/安全、日志、聊天显存守卫、前端显示共用同一个根对象
	System SystemSettings `json:"system"`
	// 月华 Agent：多模态输入方式 + 追加设定（模型路由为本地固定值，不开放配置）
	Agent struct {
		// 语音识别模型名称（ASR）
		ASRModel string `json:"asr_model"`
		// 静态图思维链形态：url=视觉读取端点 URL（默认）/ base64=变换管线直出 data URI（云端服务用）
		ImageInput string `json:"image_input"`
		// 视觉读取副本最大边长（像素），默认 1080
		VisionMaxSide int `json:"vision_max_side"`
		// 视觉副本缓存池容量上限（MB），超限按最久未用淘汰，默认 512
		VisionCacheCapMB int `json:"vision_cache_cap_mb"`
		// 追加设定：逐行附加到「月华 · 对话者」系统提示词末尾（由 Go 思维链读取；
		// 未配置或空数组时不追加任何内容）
		AppendSettings []string `json:"append_settings"`
	} `json:"agent"`
	// 网络搜索智能体（web_search）：检索词提炼、统一摘要与浏览器滚动策略。
	// 仅由搜索链（chain/search → 网页抓取执行体 executors/WebCrawler）消费，与其余配置节统一在此解析，
	// 智能体侧只读取解析后的全局值，不再自行读取 config/general.json。
	WebSearch struct {
		MaxPages        int   `json:"max_pages"`               // 页面条目总数：首页恒占 1 条，其余为同时打开的详情页
		MaxResults      int   `json:"max_results"`             // 结果页解析的检索结果条数
		SummaryMaxChars int   `json:"summary_max_chars"`       // 统一摘要硬切断字符数
		MaxRunSeconds   int   `json:"max_run_seconds"`         // 单次检索总时长软上限（秒）
		CacheEnabled    *bool `json:"cache_enabled,omitempty"` // 近期访问标记开关（未配置默认开启）
		CacheTTLDays    int   `json:"cache_ttl_days"`          // 近期访问标记的有效期（天）
		TextTotalChars  int   `json:"text_total_chars"`        // 全部页面正文的总提取硬上限（由所有页面均分）
		ScrollSettleMS  int   `json:"scroll_settle_ms"`        // 每次滚动后的等待毫秒数
		ScrollMaxSteps  int   `json:"scroll_max_steps"`        // 单页滚动步数上限
	} `json:"web_search"`
	// 文献解析智能体（document_reader）：大文件轻量扫描通道。
	// 主模型只产出扫描计划，轻量模型逐片做定向/通用摘要，主模型再归并并按需回读原文。
	DocumentReader struct {
		ScanModel      string `json:"scan_model"`           // 轻量扫描使用的模型名
		SwapModel      *bool  `json:"swap_model,omitempty"` // 是否卸载主模型、加载轻量模型（未配置默认开启）
		ScanThreshold  int    `json:"scan_threshold"`       // 启用轻量扫描的字符阈值
		ScanMaxChunks  int    `json:"scan_max_chunks"`      // 单次扫描最多处理的片数
		ScanChunkRunes int    `json:"scan_chunk_runes"`     // 扫描单片字符上限
	} `json:"document_reader"`
	// 记忆库不再有独立的模型路由：嵌入与多模态模型统一复用 agent 分组
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
	// Gitee Release 发布通道不出现在 general.json，也不由主程序读取：
	// 它属于独立更新器，配置在 local_data/config/release.json（含令牌，已 gitignore）。
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
	// 本地固定模型端点按 BasicPort 同步（包初始化时端口标志尚未解析）
	AgentEmbeddingURL = LlamaV1URL()
	AgentMultimodalURL = LlamaV1URL()
	// 获取当前可执行文件的路径
	exePath, err := os.Executable()
	// 若获取失败，打印错误日志并直接返回
	if err != nil {
		log.Printf("[Config][ERROR] -> 获取可执行文件路径失败: %v", err)
		return
	}
	// 提取可执行文件所在的目录
	exeDir := filepath.Dir(exePath)
	// 拼接公共配置文件 config/general.json 的完整路径
	configPath := filepath.Join(exeDir, LocalDir, "config", "general.json")
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
	// 解析结果 → 全局配置（映射规则集中在 applyConfig，便于单测覆盖）
	applyConfig(parameter)

	// ==== 绘图配置：唯一来源 local_data/config/painter.json（扁平结构，general.json 不再承载绘图字段） ====
	if fileCfg, ok, perr := LoadPainterConfigFile(filepath.Join(exeDir, LocalDir, "config")); perr != nil {
		log.Printf("[Config][ERROR] -> 解析 %s 失败，绘图配置保持默认值: %v", PainterConfigFile, perr)
	} else if ok {
		applyPainterConfig(fileCfg)
	} else {
		log.Printf("[Config][ERROR] -> 未找到 local_data/config/%s，绘图配置保持默认值", PainterConfigFile)
	}
}

// applyConfig 把解析结果写入全局配置。
//
// 独立成函数：配置文件读的是 exe 同级的 config/general.json，init 的文件路径无法在测试中
// 复现，而「字段缺省保持默认」这类规则必须被单测钉住——否则改动一处就会静默失效
// （配置回到默认值，且没有任何报错）。
func applyConfig(parameter *ModelConfig) {
	// ==== 核心智能体配置（agent） ====
	// 模型路由（embedding / multimodal 的 url、model、key）已收敛为本地固定参数，
	// 不再从配置文件或命令行开放；旧配置中的对应字段在 JSON 解码时被忽略。
	if parameter.Agent.ASRModel != "" {
		AgentASRModel = parameter.Agent.ASRModel
	}
	// Go 原生思维链直接消费的追加设定
	AgentAppendSettings = append([]string(nil), parameter.Agent.AppendSettings...)
	// ==== 静态图思维链形态（agent.image_input / agent.vision_max_side） ====
	// url=视觉读取端点（默认）；base64=视觉读取管线直出的副本（供无法回取本机 URL 的云端模型）。
	// 两种形态都是 ≤ vision_max_side 的副本，原图永远不进入思维链。
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
	// 渲染器枚举：仅接受 bedrock / live2d / gif，未配置保持默认 bedrock，非法取值告警并保留默认
	switch parameter.System.CharacterRenderer {
	case "bedrock", "live2d", "gif":
		CharacterRenderer = parameter.System.CharacterRenderer
	case "":
		// 未配置时保持默认渲染器
	default:
		log.Printf("[Config][WARN] -> system.character_renderer 取值非法: %q（允许 bedrock/live2d/gif），保持默认 %s", parameter.System.CharacterRenderer, CharacterRenderer)
	}
	if parameter.System.Live2DModel != "" {
		Live2DModel = parameter.System.Live2DModel
	}
	// ==== 网络搜索智能体（web_search） ====
	// 搜索模型固定为 light_reasoning、并在搜索前卸载主模型，属流程内建行为，不再走配置。
	// 未配置或非法（<=0）的数值字段沿用默认值；布尔字段仅在显式给出时覆盖。
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
	if parameter.WebSearch.TextTotalChars > 0 {
		WebSearchTextTotalChars = parameter.WebSearch.TextTotalChars
	}
	if parameter.WebSearch.ScrollSettleMS > 0 {
		WebSearchScrollSettleMS = parameter.WebSearch.ScrollSettleMS
	}
	if parameter.WebSearch.ScrollMaxSteps > 0 {
		WebSearchScrollMaxSteps = parameter.WebSearch.ScrollMaxSteps
	}
	// ==== 文献解析智能体（document_reader） ====
	if parameter.DocumentReader.ScanModel != "" {
		DocumentScanModel = parameter.DocumentReader.ScanModel
	}
	if v := parameter.DocumentReader.SwapModel; v != nil {
		b := *v
		DocumentScanSwap = &b
	}
	if parameter.DocumentReader.ScanThreshold > 0 {
		DocumentScanThreshold = parameter.DocumentReader.ScanThreshold
	}
	if parameter.DocumentReader.ScanMaxChunks > 0 {
		DocumentScanMaxChunks = parameter.DocumentReader.ScanMaxChunks
	}
	if parameter.DocumentReader.ScanChunkRunes > 0 {
		DocumentScanChunkRunes = parameter.DocumentReader.ScanChunkRunes
	}
	// 收敛到轻量扫描可接受的边界：片数过多会拖长单次任务，
	// 单片过大则接近 32768 上下文上限（中文约 1 字 1 token）。
	if DocumentScanThreshold < 10000 {
		DocumentScanThreshold = 10000
	}
	if DocumentScanMaxChunks < 5 {
		DocumentScanMaxChunks = 5
	}
	if DocumentScanMaxChunks > 200 {
		DocumentScanMaxChunks = 200
	}
	if DocumentScanChunkRunes < 4000 {
		DocumentScanChunkRunes = 4000
	}
	if DocumentScanChunkRunes > 16000 {
		DocumentScanChunkRunes = 16000
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
	// 启动时自动启动 HTTPS 桥接：未配置保持关闭（默认不对外暴露代理端口）
	if parameter.System.AutoHTTPSProxy != nil {
		AutoHTTPSProxy = *parameter.System.AutoHTTPSProxy
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
	// ==== 聊天显存守卫与 QQ 桥接 ====
	// chat（system.chat_vram_guard 聊天显存守卫）仅由月华 TypeScript 读取，
	// bridging（QQ 桥接）由 lunar_astral/bridging/napcat 自行解析，均不落 Go 全局。
}
