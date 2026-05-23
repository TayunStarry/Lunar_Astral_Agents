package module

import (
	"sync"
	"sync/atomic"
)

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-vulkan/ggml-vulkan.a" "C:/VulkanSDK/1.4.350.0/Lib/libvulkan.dll.a" -lstdc++ -lpthread
#cgo CFLAGS: -I"D:/TTS/qwen3-tts.cpp-main/src" -I"D:/TTS/qwen3-tts.cpp-main/ggml/include"
#include <stdlib.h>
#include "qwen3tts_c_api.h"

//export streamPCMCallback
extern int streamPCMCallback(float* samples, int32_t n_samples, int32_t sample_rate, int is_final, void* user_data);
*/
import "C"

// TTSEngine 用于存储TTS引擎实例
type TTSEngine struct {
	// handle 用于存储TTS引擎实例的句柄
	handle *C.Qwen3Tts
	// modelDir 用于指定模型目录
	modelDir string
	// refAudio 用于指定参考音频文件
	refAudio string
	// mu 用于保护TTSEngine的互斥锁
	mu sync.Mutex
	// languageID 用于指定语言ID
	languageID int32
}

// speakerEmbedCache 用于存储语音特征的缓存
type speakerEmbedCache struct {
	// mu 用于保护speakerEmbedCache的互斥锁
	mu sync.Mutex
	// embeddings 用于存储语音特征的缓存
	embeddings map[string][]float32
	// fileHashes 用于存储参考音频文件的 SHA256 哈希，用于校验缓存一致性
	fileHashes map[string]string
	// cacheDir 用于指定缓存目录
	cacheDir string
}

// StreamPCMChunk 用于存储PCM音频数据
type StreamPCMChunk struct {
	// Samples 用于存储PCM音频数据
	Samples []float32
	// SampleRate 用于存储采样率
	SampleRate int32
	// IsFinal 用于存储是否为最后一帧音频
	IsFinal bool
}

// streamingContext 用于存储流式上下文
type streamingContext struct {
	// ch 用于存储PCM音频数据的通道
	ch chan StreamPCMChunk
	// done 用于存储流式上下文是否完成的通道
	done chan struct{}
	// err 用于存储流式上下文的错误信息
	err error
	// abort 用于存储流式上下文是否被取消
	abort atomic.Int32
}

// TTSRequest 用于存储TTS请求
type TTSRequest struct {
	// Text 用于存储要转换的文本
	Text string `json:"text"`
	// RefAudio 用于存储参考音频文件
	RefAudio string `json:"ref_audio,omitempty"`
	// LanguageID 用于存储语言ID
	LanguageID int32 `json:"language_id,omitempty"`
	// Temperature 用于控制生成随机性
	Temperature float32 `json:"temperature,omitempty"`
	// TopK 用于Top-K采样
	TopK int32 `json:"top_k,omitempty"`
	// TopP 用于Top-P采样
	TopP float32 `json:"top_p,omitempty"`
	// MaxTokens 用于设置最大生成token数
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// RepetitionPenalty 用于控制重复惩罚
	RepetitionPenalty float32 `json:"repetition_penalty,omitempty"`
	// Threads 用于设置线程数
	Threads int32 `json:"threads,omitempty"`
}

// TTSResponse 用于存储TTS响应
type TTSResponse struct {
	// Success 用于存储是否成功
	Success bool `json:"success"`
	// Audio 用于存储音频文件路径
	Audio string `json:"audio,omitempty"`
	// Error 用于存储错误信息
	Error string `json:"error,omitempty"`
}

// WSStreamRequest 用于存储WebSocket流请求
type WSStreamRequest struct {
	// Text 用于存储要转换的文本
	Text string `json:"text"`
	// RefAudio 用于存储参考音频文件
	RefAudio string `json:"ref_audio,omitempty"`
	// LanguageID 用于存储语言ID
	LanguageID int32 `json:"language_id,omitempty"`
	// ChunkFrames 用于存储每帧音频的样本数
	ChunkFrames int32 `json:"chunk_frames,omitempty"`
	// Temperature 用于控制生成随机性
	Temperature float32 `json:"temperature,omitempty"`
	// TopK 用于Top-K采样
	TopK int32 `json:"top_k,omitempty"`
	// TopP 用于Top-P采样
	TopP float32 `json:"top_p,omitempty"`
	// MaxTokens 用于设置最大生成token数
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// RepetitionPenalty 用于控制重复惩罚
	RepetitionPenalty float32 `json:"repetition_penalty,omitempty"`
	// Threads 用于设置线程数
	Threads int32 `json:"threads,omitempty"`
}

// WSStreamResponse 用于存储WebSocket流响应
type WSStreamResponse struct {
	// Type 用于存储响应类型
	Type string `json:"type"`
	// Audio 用于存储音频文件路径
	Audio string `json:"audio,omitempty"`
	// ChunkIndex 用于存储音频块序号
	ChunkIndex int32 `json:"chunk_index,omitempty"`
	// TotalChunks 用于存储总块数（final时提供）
	TotalChunks int32 `json:"total_chunks,omitempty"`
	// TotalSamples 用于存储总样本数
	TotalSamples int32 `json:"total_samples,omitempty"`
	// SampleRate 用于存储采样率
	SampleRate int32 `json:"sample_rate,omitempty"`
	// IsFinal 用于存储是否为最后一帧音频
	IsFinal bool `json:"is_final,omitempty"`
	// Error 用于存储错误信息
	Error string `json:"error,omitempty"`
}
