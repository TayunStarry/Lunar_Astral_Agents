package ttsrouter

import KokoroTTS "CorePersonality/engine/kokoro_tts"

// TTSRequest 统一参数样式：月华语音合成的唯一请求形态。
//
// 参数层面统一：所有引擎共用同一套字段。lunar_config.json「tts」节仅作默认值
// 来源——请求未携带的字段回退配置默认值，请求显式提供的值优先覆写配置。
// 其中需区分「未携带」与「显式零值」的三个字段（speed/seed/disable_postproc）
// 采用指针类型：nil 表示未携带（用配置默认），非 nil 表示显式覆写（即使为 0/false）；
// 字符串与列表字段以空值表示未携带。
// 执行层面按所选引擎自身格式适配（Synthesize 内部转换），引擎不支持的
// 参数自动忽略——例如 Kokoro 不支持参考音频，RefAudio/RefText 会被丢弃。
type TTSRequest struct {
	// Text 要合成的文本（公共必填）
	Text string `json:"text"`

	// Engine 引擎覆写（Qwen/Kokoro/OmniVoice）；空或非法取值时回退 tts.engine 配置
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
	Mix []KokoroTTS.MixVoice `json:"mix,omitempty"`
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
