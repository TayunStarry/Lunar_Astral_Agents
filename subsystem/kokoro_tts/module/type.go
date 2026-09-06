package module

import (
	"sync"

	"github.com/yalue/onnxruntime_go"
)

// Tokenizer 音素词表分词器（字符级 vocab）
type Tokenizer struct {
	// vocab 字符到 token id 的映射
	vocab map[rune]int64
}

// Voice 音色数据（[510, 256] float32，每行对应一个音素数量的风格向量）
type Voice struct {
	// Name 音色名称（不含 .bin 后缀）
	Name string
	// Rows 音色行数据，共 510 行，每行 256 维
	Rows [][]float32
}

// Engine Kokoro TTS 引擎
type Engine struct {
	// session ONNX Runtime 动态会话
	session *onnxruntime_go.DynamicAdvancedSession
	// tokenizer 音素分词器
	tokenizer *Tokenizer
	// voices 音色映射表（名称 -> 音色）
	voices map[string]*Voice
	// voiceOrder 音色名称的有序列表
	voiceOrder []string
	// mu 保护合成过程的互斥锁（onnx 会话非并发安全）
	mu sync.Mutex
	// zhFront 中文前端实例
	zhFront *zhFrontend
	// espeakAvailable 英文 espeak-ng 是否可用
	espeakAvailable bool
}

// TTSRequest 语音合成请求
type TTSRequest struct {
	// Text 要合成的文本
	Text string `json:"text"`
	// Voice 音色名称（如 zf_001 / zm_031 / af_maple），为空则使用默认音色
	Voice string `json:"voice,omitempty"`
	// Speed 语速倍率（0.5 ~ 2.0），默认为 1.0
	Speed float32 `json:"speed,omitempty"`
	// Lang 语言强制指定（zh / en / auto），默认为 auto 自动识别
	Lang string `json:"lang,omitempty"`
}

// TTSResponse 语音合成响应
type TTSResponse struct {
	// Success 是否成功
	Success bool `json:"success"`
	// Audio base64 编码的 WAV 音频数据
	Audio string `json:"audio,omitempty"`
	// Phonemes 合成使用的音素序列（调试用）
	Phonemes string `json:"phonemes,omitempty"`
	// Voice 实际使用的音色名称
	Voice string `json:"voice,omitempty"`
	// SampleRate 音频采样率
	SampleRate int `json:"sample_rate"`
	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// VoiceInfo 音色信息
type VoiceInfo struct {
	// Name 音色名称
	Name string `json:"name"`
	// Lang 音色所属语言（zf/zm 为中文，af/bf 为英文）
	Lang string `json:"lang"`
}

// DictEntry 读音词典词条
type DictEntry struct {
	// Word 词语
	Word string `json:"word"`
	// Pinyin 带调拼音（音节用空格分隔，如 "hang2 zhang3"）
	Pinyin string `json:"pinyin"`
}

// DictRequest 读音词典操作请求
type DictRequest struct {
	// Word 词语
	Word string `json:"word"`
	// Pinyin 带调拼音
	Pinyin string `json:"pinyin"`
}

// GuessResponse 读音查询响应
type GuessResponse struct {
	// Success 请求是否成功
	Success bool `json:"success"`
	// Word 查询的词语
	Word string `json:"word"`
	// Pinyin 当前会使用的带调拼音（词典优先）
	Pinyin string `json:"pinyin"`
	// InDict 是否命中用户词典
	InDict bool `json:"in_dict"`
}
