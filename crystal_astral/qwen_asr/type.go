package qwen_asr

import (
	"sync"
	"unsafe"
)

// QwenASR Qwen3-ASR 语音识别引擎
type QwenASR struct {
	ctx      unsafe.Pointer // 引擎上下文句柄（C qwen_ctx_t*）
	mu       sync.Mutex     // 串行化推理（C 侧上下文非并发安全）
	modelDir string         // 模型目录（仅用于日志）
}

// AsrResponse ASR 识别响应
type AsrResponse struct {
	Status      string  `json:"status"`       // 识别状态：success / error
	Confidence  float64 `json:"confidence"`   // 由文本长度估算的置信度
	Text        string  `json:"text"`         // 识别文本
	Error       string  `json:"error,omitempty"` // 错误信息
	AudioFormat string  `json:"audio_format"` // 实际送入引擎的音频格式
}
