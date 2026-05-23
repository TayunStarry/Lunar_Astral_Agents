package tts

import (
	"github.com/gorilla/websocket"
	"net/http"
)

// ttsWrapperCacheMax 是TTS缓存的最大条目数。
// 超过这个数量的条目会被移除缓存。
const ttsWrapperCacheMax = 15

// ttsStreamUpgrader 是TTS流的升级器，用于将HTTP请求升级为WebSocket连接。
var ttsStreamUpgrader = websocket.Upgrader{
	// ReadBufferSize 是读取缓冲区的大小，用于存储读取的HTTP请求数据。
	ReadBufferSize: 1024,
	// WriteBufferSize 是写入缓冲区的大小，用于存储写入的HTTP响应数据。
	WriteBufferSize: 65536,
	// CheckOrigin 是一个检查函数，用于验证请求的来源是否安全。
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// ttsWrapperCache 是TTS缓存，用于存储已生成的音频数据。
var ttsWrapperCache = &ttsCache{
	// items 是缓存中的条目映射，键为文本，值为条目指针。
	items: make(map[string]*ttsCacheEntry),
	// order 是缓存中条目的顺序，用于维护缓存的LRU策略。 最近使用的条目在最前面，最久未使用的条目在最后面。
	order: make([]string, 0, ttsWrapperCacheMax),
}
