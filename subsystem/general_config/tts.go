package GeneralConfig

import "flag"

// ==== 月华语音合成（TTS）全局配置 ====
//
// 引擎选择与统一效果参数，默认值经 lunar_config.json 的 tts 节覆盖（见 init.go）。
// 参数层面统一：所有引擎共用同一套字段；执行层面由 engine/tts_router 按引擎自身
// 格式适配，引擎不支持的参数（如 Kokoro 不支持参考音频）自动忽略。

var (
	// TTSEngine 语音合成引擎枚举（Qwen / Kokoro / OmniVoice）
	TTSEngine = flag.String("tts-engine", "Qwen", "月华语音合成引擎（Qwen/Kokoro/OmniVoice）")
	// TTSRefAudio 参考音频路径（Qwen/OmniVoice 声音克隆；Kokoro 不支持参考音频，忽略）
	TTSRefAudio = flag.String("tts-ref-audio", "", "TTS 参考音频路径（Qwen/OmniVoice）")
	// TTSRefText 参考音频转写文本（OmniVoice 声音克隆必填；其余忽略）
	TTSRefText = flag.String("tts-ref-text", "", "TTS 参考音频转写文本（OmniVoice）")
	// TTSVoice 音色名称（Kokoro 专有；其余忽略）
	TTSVoice = flag.String("tts-voice", "", "TTS 音色名称（Kokoro）")
	// TTSSpeed 语速倍率（Kokoro 0.5~2.0；其余忽略）
	TTSSpeed = flag.Float64("tts-speed", 1.0, "TTS 语速倍率（Kokoro）")
	// TTSLang 语言提示（Kokoro: zh/en/auto；OmniVoice: 语言提示；Qwen 忽略）
	TTSLang = flag.String("tts-lang", "", "TTS 语言提示")
	// TTSInstruct 音色属性关键词（OmniVoice 专有，与参考音频互斥；其余忽略）
	TTSInstruct = flag.String("tts-instruct", "", "TTS 音色属性关键词（OmniVoice）")
	// TTSSeed 随机种子（OmniVoice，0 使用库默认；其余忽略）
	TTSSeed = flag.Uint64("tts-seed", 0, "TTS 随机种子（OmniVoice）")
	// TTSDisablePostproc 关闭输出后处理（OmniVoice；其余忽略）
	TTSDisablePostproc = flag.Bool("tts-disable-postproc", false, "关闭 TTS 输出后处理（OmniVoice）")
)

// TTSMixEntry 音色混合分量（Kokoro 专有；其余忽略）
type TTSMixEntry struct {
	// Voice 音色名称
	Voice string `json:"voice"`
	// Weight 融合权重（相对权重，引擎自动归一化；0 按等权处理）
	Weight float64 `json:"weight"`
}

// TTSMix 音色混合列表：数组无法经命令行 flag 覆盖，仅由配置文件填充
var TTSMix []TTSMixEntry
