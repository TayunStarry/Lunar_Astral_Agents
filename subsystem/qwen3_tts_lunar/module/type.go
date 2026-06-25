package module

import (
	"container/list"
	"sync"
	"sync/atomic"
)


// CacheEntry 缓存条目，存储音频数据及其在LRU链表中的位置
type CacheEntry struct {
	// key 用于存储缓存键
	key string
	// audioData 用于存储base64编码的WAV音频数据
	audioData string
	// element 用于存储在LRU链表中的位置
	element *list.Element
}

// InflightCall 用于防止缓存击穿的单飞调用，多个并发请求共享同一个合成结果
// 使用 close(done) 模式支持多个等待者同时等待
type InflightCall struct {
	// mu 用于保护 result/err/completed 字段的并发访问
	mu sync.Mutex
	// result 用于存储合成结果（base64编码的WAV音频）
	result string
	// err 用于存储合成过程中的错误
	err error
	// completed 标记合成是否已完成
	completed bool
	// done 用于通知所有等待者合成已完成
	done chan struct{}
}

// TTSCache 线程安全的LRU缓存，基于哈希表+双向链表实现
// 使用sync.RWMutex保证并发安全，通过singleflight模式防止缓存击穿
type TTSCache struct {
	// mu 用于保护缓存并发访问的读写锁
	mu sync.RWMutex
	// capacity 用于指定缓存容量上限
	capacity int
	// items 用于存储缓存键到条目的映射
	items map[string]*CacheEntry
	// lruList 用于按访问顺序组织的双向链表（头部为最近访问）
	lruList *list.List
	// inflight 用于存储正在合成中的单飞调用
	inflight map[string]*InflightCall

	// 统计信息（原子操作，无需加锁）
	// hits 用于记录缓存命中次数
	hits atomic.Int64
	// misses 用于记录缓存未命中次数
	misses atomic.Int64
	// evictions 用于记录LRU淘汰次数
	evictions atomic.Int64
	// updates 用于记录缓存更新次数
	updates atomic.Int64
}
