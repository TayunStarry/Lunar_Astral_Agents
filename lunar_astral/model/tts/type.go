package tts

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-vulkan/ggml-vulkan.a" "C:/VulkanSDK/1.4.350.0/Lib/libvulkan.dll.a" -lstdc++ -lpthread
#cgo CFLAGS: -I"D:/TTS/qwen3-tts.cpp-main/src" -I"D:/TTS/qwen3-tts.cpp-main/ggml/include"
#include <stdlib.h>
#include "qwen3tts_c_api.h"
*/
import "C"

import (
	"sync"
)

// TTSEngine 是一个 TTS 引擎，用于生成语音。
type TTSEngine struct {
	handle     *C.Qwen3Tts
	modelDir   string
	refAudio   string
	mu         sync.Mutex
	languageID int32
}

// cacheEntry 是缓存中的一个条目，用于存储已生成的语音。
type cacheEntry struct {
	audio string
	ready chan struct{}
}

// speakerEmbedCache 是 speaker embedding 的本地持久化缓存。
type speakerEmbedCache struct {
	mu         sync.Mutex
	embeddings map[string][]float32 // refAudio path -> speaker embedding
	cacheDir   string
}

// TTSCache 是一个缓存，用于存储已生成的语音。
type TTSCache struct {
	mu    sync.Mutex
	items map[string]*cacheEntry
	order []string
}

// TTSRequest 是一个 TTS 请求，包含要合成的文本。
type TTSRequest struct {
	// Text 是要合成的文本。
	Text string `json:"text"`
}

// TTSResponse 是一个 TTS 响应，包含合成的语音或错误信息。
type TTSResponse struct {
	// Success 表示请求是否成功。
	Success bool `json:"success"`
	// Audio 是合成的语音，Base64 编码。
	Audio string `json:"audio,omitempty"`
	// Error 是请求失败时的错误信息。
	Error string `json:"error,omitempty"`
}

// WSStreamRequest 是 WebSocket 流式 TTS 请求结构。
type WSStreamRequest struct {
	// Text 是要合成的文本。
	Text string `json:"text"`
	// RefAudio 是参考音频路径（可选）。
	RefAudio string `json:"ref_audio,omitempty"`
	// LanguageID 是语言标识符（可选）。
	LanguageID int32 `json:"language_id,omitempty"`
	// ChunkFrames 是每个块累积的帧数（可选）。
	ChunkFrames int32 `json:"chunk_frames,omitempty"`
}

// WSStreamResponse 是 WebSocket 流式 TTS 响应结构。
type WSStreamResponse struct {
	// Type 是消息类型。
	Type string `json:"type"`
	// Audio 是 Base64 编码的 PCM16 音频数据。
	Audio string `json:"audio,omitempty"`
	// TotalSamples 是累计采样数。
	TotalSamples int32 `json:"total_samples,omitempty"`
	// SampleRate 是采样率。
	SampleRate int32 `json:"sample_rate,omitempty"`
	// IsFinal 表示是否为最终消息。
	IsFinal bool `json:"is_final,omitempty"`
	// Error 是错误信息。
	Error string `json:"error,omitempty"`
}

// StreamPCMChunk 是流式 PCM 音频块结构。
type StreamPCMChunk struct {
	// Samples 是浮点采样数据。
	Samples []float32
	// SampleRate 是采样率。
	SampleRate int32
	// IsFinal 表示是否为最终块。
	IsFinal bool
}

// streamCacheEntry 是流式缓存中的一个条目
type streamCacheEntry struct {
	// Text 是要合成的文本。
	text string
	// RefAudio 是参考音频路径（可选）。
	refAudio string
	// LanguageID 是语言标识符（可选）。
	languageID int32
	// ChunkFrames 是每个块累积的帧数（可选）。
	chunkFrames int32
	// Subscribers 是订阅者通道列表。
	subscribers []chan StreamPCMChunk
	// Done 是完成通道，用于通知订阅者请求已完成。
	done chan struct{}
	// Err 是请求失败时的错误信息。
	err error
	// SampleRate 是采样率。
	sampleRate int32
	// Mu 是互斥锁，用于保护缓存项的并发访问。
	mu sync.Mutex
	// Aborted 是一个整数，用于表示请求是否被取消。
	aborted int32
}
