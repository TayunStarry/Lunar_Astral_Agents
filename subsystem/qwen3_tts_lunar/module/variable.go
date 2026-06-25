package module

import (
	"sync"
)

// 全局TTS引擎实例
var globalTTS *TTSEngine

// 用于确保TTS引擎实例只被一次初始化
var ttsOnce sync.Once

// 用于存储语音特征的缓存
var embedCache = &speakerEmbedCache{
	// embeddings 用于存储语音特征的缓存
	embeddings: make(map[string][]float32),
	// fileHashes 用于存储参考音频文件的 SHA256 哈希
	fileHashes: make(map[string]string),
	// cacheDir 用于指定缓存目录
	cacheDir: "./local_data/audios/cache",
}

// MaxCacheSize TTS音频缓存容量上限（条目数）
const MaxCacheSize = 64

// 全局TTS音频缓存实例，容量为 MaxCacheSize 条记录
var ttsCache = NewTTSCache(MaxCacheSize)

// synthFunc 用于TTS合成的函数变量，可被测试替换为mock实现
var synthFunc = SynthesizeText
