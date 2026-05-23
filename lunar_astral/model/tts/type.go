package tts

import (
	"bytes"
	"net/http"
	"sync"
)

// ttsCacheEntry 是TTS缓存中的一个条目，包含音频数据和一个准备信号通道。
type ttsCacheEntry struct {
	// audio 是音频数据的字符串表示。
	audio string
	// ready 是一个通道，用于通知调用者音频数据已准备就绪。
	ready chan struct{}
}

// ttsCache 是TTS缓存，用于存储已生成的音频数据。
type ttsCache struct {
	// mu 是缓存的互斥锁，用于保护缓存的并发访问。
	mu sync.Mutex
	// items 是缓存中的条目映射，键为文本，值为条目指针。
	items map[string]*ttsCacheEntry
	// order 是缓存中条目的顺序，用于维护缓存的LRU策略。 最近使用的条目在最前面，最久未使用的条目在最后面。
	order []string
}

// responseCapture 是一个模拟的http.ResponseWriter，用于捕获HTTP响应。
type responseCapture struct {
	// ResponseWriter 是http.ResponseWriter的实现，用于写入响应体。
	http.ResponseWriter
	// body 是响应体的缓冲区，用于存储写入的响应数据。
	body bytes.Buffer
	// statusCode 是响应状态码。
	statusCode int
}

// ttsMockWriter 是一个模拟的http.ResponseWriter，用于捕获TTS响应。
type ttsMockWriter struct {
	// body 是响应体的缓冲区，用于存储写入的响应数据。
	body bytes.Buffer
	// statusCode 是响应状态码。
	statusCode int
}
