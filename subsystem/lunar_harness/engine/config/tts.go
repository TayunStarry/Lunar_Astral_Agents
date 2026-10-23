package config

// ==== 月华语音合成（TTS）全局配置 ====

var (
	// TTSEngine 语音合成引擎枚举（Qwen / Kokoro / OmniVoice）
	TTSEngine = "Qwen"
	// TTSRefAudio 参考音频路径（Qwen/OmniVoice）
	TTSRefAudio = ""
	// TTSRefText 参考音频转写文本（OmniVoice）
	TTSRefText = ""
	// TTSVoice 音色名称（Kokoro）
	TTSVoice = ""
	// TTSSpeed 语速倍率（Kokoro）
	TTSSpeed = 1.0
	// TTSLang 语言提示
	TTSLang = ""
	// TTSInstruct 音色属性关键词（OmniVoice）
	TTSInstruct = ""
	// TTSSeed 随机种子（OmniVoice）
	TTSSeed = uint64(0)
	// TTSDisablePostproc 关闭输出后处理（OmniVoice）
	TTSDisablePostproc = false
	// TTSIdleUnloadMinutes 引擎闲置卸载阈值（分钟）
	TTSIdleUnloadMinutes = 5
)

// TTSMixEntry 音色混合分量（Kokoro 专有；其余忽略）
type TTSMixEntry struct {
	Voice  string  `json:"voice"`
	Weight float64 `json:"weight"`
}

// TTSMix 音色混合列表（仅由配置文件填充）
var TTSMix []TTSMixEntry
