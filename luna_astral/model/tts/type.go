package tts

/*
#cgo LDFLAGS: -L"D:/TTS/qwen3-tts.cpp-main/build" -L"D:/TTS/qwen3-tts.cpp-main/ggml/build/src" "D:/TTS/qwen3-tts.cpp-main/build/libqwen3tts.dll.a" -lqwen3_tts -ltts_transformer -ltext_tokenizer -laudio_tokenizer_encoder -laudio_tokenizer_decoder "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-base.a" "D:/TTS/qwen3-tts.cpp-main/ggml/build/src/ggml-cpu.a" -lstdc++ -lpthread
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
