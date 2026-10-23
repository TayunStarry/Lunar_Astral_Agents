package qwentts

import (
	"sync"
	"sync/atomic"
	"time"
)

// 全局TTS引擎实例
var globalTTS *TTSEngine

// ttsMu 引擎生命周期锁：
//   - 写锁（Lock）串行化加载（EnsureLoaded）与卸载（UnloadTTSEngine）
//   - 读锁（RLock）在合成期间钉住引擎实例，防止合成中途引擎被卸载
var ttsMu sync.RWMutex

// ttsLoaded 引擎是否已加载（ttsMu 写锁内访问）
var ttsLoaded bool

// lastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成；供闲置卸载守护判断）
var lastUsedUnix atomic.Int64

// ttsIdleUnloadSeconds 引擎闲置卸载阈值：5 分钟无合成调用即卸载
const ttsIdleUnloadSeconds = 300

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

// touchEngine 更新最近合成时间戳（合成入口调用）
func touchEngine() {
	lastUsedUnix.Store(time.Now().Unix())
}

// LastUsedUnix 最近一次合成调用的 unix 秒（0 = 从未合成）
func LastUsedUnix() int64 {
	return lastUsedUnix.Load()
}
