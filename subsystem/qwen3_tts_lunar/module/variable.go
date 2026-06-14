package module

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
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

// 用于存储流式上下文的映射
var streamCtxMap = make(map[int32]*streamingContext)

// 用于生成唯一的流式上下文ID
var streamCtxCounter int32

// 用于保护streamCtxMap	的互斥锁
var streamCtxMapMu sync.Mutex

// 用于升级WebSocket连接的实例
var wsUpgrader = websocket.Upgrader{
	// ReadBufferSize 用于设置读取缓冲区大小
	ReadBufferSize: 1024,
	// WriteBufferSize 用于设置写入缓冲区大小
	WriteBufferSize: 65536,
	// CheckOrigin 用于检查WebSocket连接的来源是否安全
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}
