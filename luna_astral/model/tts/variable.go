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

// streamCtxMap 是流式上下文映射，键为上下文 ID，值为流式上下文。
var streamCtxMap = make(map[int32]*StreamingContext)

// streamCtxMapMu 是保护流式上下文映射的互斥锁。
var streamCtxMapMu sync.Mutex

// streamCtxCounter 是流式上下文计数器，用于生成唯一的上下文 ID。
var streamCtxCounter int32
