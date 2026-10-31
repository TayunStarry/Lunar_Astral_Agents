package broadcaster

import (
	"container/list"
	"sync"
	"sync/atomic"
)

// ==== 统一请求样式（Broadcaster 设计语言 · 唯一请求形态） ====
//
// Broadcaster 是月华语音播报的统一链路：全部 TTS 调用（HTTP 端点、JS 运行时
// textToSpeech 绑定、LTP9 包工具）强制收敛到本包 /broadcaster 端点族，单一合成
// 入口按请求 engine 参数（缺省 tts.engine 配置，默认 Qwen）选择引擎执行。
//
// 参数层面统一：所有引擎共用同一套字段。lunar_config.json「tts」节仅作默认值
// 来源——请求未携带的字段回退配置默认值，请求显式提供的值优先覆写配置。
// 其中需区分「未携带」与「显式零值」的三个字段（speed/seed/disable_postproc）
// 采用指针类型：nil 表示未携带（用配置默认），非 nil 表示显式覆写（即使为 0/false）；
// 字符串与列表字段以空值表示未携带。
// 执行层面按所选引擎自身格式适配（Synthesize 内部转换），引擎不支持的参数
// 自动忽略——例如 Kokoro 不支持参考音频，RefAudio/RefText 会被丢弃。
// 各引擎参数支持范围经 /broadcaster/capabilities 能力矩阵声明，调用方按能力
// 探测而非硬编码引擎差异。

// TTSRequest 统一参数样式：月华语音播报链路的唯一请求形态
type TTSRequest struct {
	// Text 要合成的文本（公共必填）
	Text string `json:"text"`

	// Engine 引擎指定（Qwen/Kokoro/OmniVoice）；空或非法取值时回退 tts.engine 配置
	Engine string `json:"engine,omitempty"`

	// ==== 声音克隆参数（Qwen / OmniVoice；Kokoro 忽略） ====
	// RefAudio 参考音频路径（相对路径按可执行文件目录解析）
	RefAudio string `json:"ref_audio,omitempty"`
	// RefText 参考音频的转写文本（OmniVoice 声音克隆必填；Qwen 忽略）
	RefText string `json:"ref_text,omitempty"`

	// ==== 音色参数（Kokoro；其余忽略） ====
	// Voice 音色名称（如 zf_001 / zm_031 / af_maple）
	Voice string `json:"voice,omitempty"`
	// Mix 音色混合列表（≥2 个时启用音色融合，优先级高于 Voice）
	Mix []MixEntry `json:"mix,omitempty"`
	// Speed 语速倍率（Kokoro 0.5 ~ 2.0；nil 表示使用配置默认值）
	Speed *float32 `json:"speed,omitempty"`
	// Lang 语言提示（Kokoro: zh/en/auto；OmniVoice: 语言提示；Qwen 忽略）
	Lang string `json:"lang,omitempty"`

	// ==== OmniVoice 专属参数（其余忽略） ====
	// Instruct 音色属性关键词（如 "female young adult moderate"，与 RefAudio 互斥）
	Instruct string `json:"instruct,omitempty"`
	// Seed 随机种子（OmniVoice，0 表示库默认；nil 表示使用配置默认值）
	Seed *uint64 `json:"seed,omitempty"`
	// DisablePostproc 关闭输出后处理（静音裁剪/峰值归一/边缘淡出）；nil 表示使用配置默认值
	DisablePostproc *bool `json:"disable_postproc,omitempty"`

	// ==== Qwen 专属采样控制（其余忽略） ====
	// LanguageID 语言 ID（0 使用引擎默认）
	LanguageID int32 `json:"language_id,omitempty"`
	// Temperature 采样温度
	Temperature float32 `json:"temperature,omitempty"`
	// TopK Top-K 采样
	TopK int32 `json:"top_k,omitempty"`
	// TopP Top-P 采样
	TopP float32 `json:"top_p,omitempty"`
	// MaxTokens 最大音频 token 数
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// RepetitionPenalty 重复惩罚
	RepetitionPenalty float32 `json:"repetition_penalty,omitempty"`
	// Threads 线程数
	Threads int32 `json:"threads,omitempty"`
	// DisableCache 禁用合成缓存
	DisableCache bool `json:"disable_cache,omitempty"`
}

// MixEntry 音色混合分量（Kokoro 专有；其余忽略）
type MixEntry struct {
	// Voice 音色名称
	Voice string `json:"voice"`
	// Weight 融合权重（相对权重，引擎自动归一化；0 按等权处理）
	Weight float64 `json:"weight"`
}

// ==== 槽位状态机（Broadcaster 设计语言 · 引擎槽位语义） ====
//
// 单载：任一时刻至多一个 TTS 引擎处于加载状态，即「被本次调用选中的引擎」；
// 切换：请求目标引擎与在位引擎不同时，按「等待在位合成完毕 → 卸载在位引擎 →
// 加载目标引擎 → 执行本次调用」的固定序列完成热切换。
// 状态经 /broadcaster/status 对外观测（零锁快照，无副作用）。

// SlotState 槽位状态（unloaded → loading → ready ⇄ synthesizing → unloading → unloaded）
type SlotState string

const (
	// SlotUnloaded 空槽：无引擎加载
	SlotUnloaded SlotState = "unloaded"
	// SlotLoading 正在加载引擎（模型权重/DLL）
	SlotLoading SlotState = "loading"
	// SlotReady 引擎就绪，可接受合成
	SlotReady SlotState = "ready"
	// SlotSynthesizing 合成进行中
	SlotSynthesizing SlotState = "synthesizing"
	// SlotUnloading 正在卸载在位引擎
	SlotUnloading SlotState = "unloading"
)

// SlotSnapshot 槽位观测快照（/broadcaster/status 与 activate/release 的响应体）
type SlotSnapshot struct {
	// Engine 当前在位引擎名（空串 = 空槽）
	Engine string `json:"engine"`
	// State 槽位状态机取值
	State SlotState `json:"state"`
	// DefaultEngine 配置的默认引擎（tts.engine，请求未携带 engine 参数时使用）
	DefaultEngine string `json:"default_engine"`
	// LastUsedUnix 槽位最近一次活动（引擎加载或合成完成）的 unix 秒（0 = 尚无活动）
	LastUsedUnix int64 `json:"last_used_unix"`
	// SwitchCount 进程内槽位切换累计次数（含空槽 → 首次加载）
	SwitchCount int64 `json:"switch_count"`
	// Version 在位引擎构建版本串（仅 OmniVoice 提供，其余为空）
	Version string `json:"version,omitempty"`
}

// EngineCapability 引擎能力声明（设计语言 · 能力矩阵）：
// 调用方经 /broadcaster/capabilities 探测各引擎支持的参数范围，而非硬编码引擎差异。
type EngineCapability struct {
	// Engine 引擎名（TTSRequest.engine 取值）
	Engine string `json:"engine"`
	// Label 引擎一句话定位
	Label string `json:"label"`
	// RefAudioClone 支持参考音频声音克隆（ref_audio）
	RefAudioClone bool `json:"ref_audio_clone"`
	// RefText 支持参考音频转写文本（ref_text）
	RefText bool `json:"ref_text"`
	// VoiceSelect 支持内置音色选择（voice）
	VoiceSelect bool `json:"voice_select"`
	// VoiceMix 支持多音色混合（mix）
	VoiceMix bool `json:"voice_mix"`
	// Speed 支持语速倍率（speed）
	Speed bool `json:"speed"`
	// Lang 支持语言提示（lang）
	Lang bool `json:"lang"`
	// Instruct 支持属性关键词设计音色（instruct）
	Instruct bool `json:"instruct"`
	// Seed 支持随机种子（seed）
	Seed bool `json:"seed"`
	// Sampling 支持采样控制（temperature/top_k/top_p/max_tokens/repetition_penalty/threads）
	Sampling bool `json:"sampling"`
	// PostprocToggle 支持输出后处理开关（disable_postproc）
	PostprocToggle bool `json:"postproc_toggle"`
	// Phonemes 成功响应携带音素序列（phonemes）
	Phonemes bool `json:"phonemes"`
	// SynthCache 内置合成缓存（disable_cache 可关闭）
	SynthCache bool `json:"synth_cache"`
	// SampleRate 输出采样率（Hz）
	SampleRate int `json:"sample_rate"`
	// AuxEndpoints 引擎专属能力端点（音色表/词典/语言表等，调用前自动完成槽位对齐）
	AuxEndpoints []string `json:"aux_endpoints"`
}

// ==== 合成缓存（Broadcaster 标准内置能力） ====
//
// 缓存位于统一合成入口层，对全部引擎一视同仁：键为「引擎 + 统一请求参数」，
// 值为 base64 WAV 音频。缓存命中不加载引擎；未命中时通过单飞调用合并并发请求，
// 合成完成后回写缓存。请求 disable_cache=true 时跳过读取，但仍会刷新缓存。

// synthCacheEntry 一条合成缓存条目（值 + LRU 链表位置）
type synthCacheEntry struct {
	// key 缓存键
	key string
	// audio base64 WAV 音频
	audio string
	// element LRU 链表节点
	element *list.Element
}

// synthInflightCall 单飞调用：并发请求共享同一次合成结果
type synthInflightCall struct {
	// mu 保护 result/err/completed
	mu sync.Mutex
	// result 合成结果（base64 WAV）
	result string
	// err 合成错误
	err error
	// completed 是否已完成
	completed bool
	// done 完成通知
	done chan struct{}
}

// SynthCache 线程安全 LRU 合成缓存（哈希表 + 双向链表 + 单飞）
type SynthCache struct {
	// mu 读写锁
	mu sync.RWMutex
	// capacity 容量上限（条目数）
	capacity int
	// items 键 → 条目
	items map[string]*synthCacheEntry
	// lruList 访问顺序链表（头部最近使用）
	lruList *list.List
	// inflight 进行中的单飞调用
	inflight map[string]*synthInflightCall

	// hits 命中次数
	hits atomic.Int64
	// misses 未命中次数
	misses atomic.Int64
	// evictions LRU 淘汰次数
	evictions atomic.Int64
}
