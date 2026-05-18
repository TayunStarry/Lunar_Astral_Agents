package tts

import "sync"

// maxCacheSize 是缓存的最大大小，用于限制缓存中存储的语音数量。
const maxCacheSize = 15

// ttsCache 是一个缓存，用于存储已生成的语音。
var ttsCache = &TTSCache{
	items: make(map[string]*cacheEntry),
	order: make([]string, 0, maxCacheSize),
}

// ttsOnce 是一个一次初始化的同步机制，用于确保 TTS 引擎只被初始化一次。
var ttsOnce sync.Once

// globalTTS 是全局的 TTS 引擎实例，用于生成语音。
var globalTTS *TTSEngine
