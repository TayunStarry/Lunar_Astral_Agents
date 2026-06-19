package tts

import (
	"container/list"
	"sync"
	"sync/atomic"
)

// MaxCacheSize 缓存容量上限
const MaxCacheSize = 5

// CacheEntry 缓存条目，存储音频数据及其在LRU链表中的位置
type CacheEntry struct {
	key       string
	audioData string // base64编码的WAV音频数据
	element   *list.Element
}

// inflightResult 单飞调用的结果
type inflightResult struct {
	data string
	err  error
}

// InflightCall 用于防止缓存击穿的单飞调用，多个并发请求共享同一个合成结果
type InflightCall struct {
	result chan inflightResult
}

// TTSCache 线程安全的LRU缓存，基于哈希表+双向链表实现
// 使用sync.RWMutex保证并发安全，通过singleflight模式防止缓存击穿
type TTSCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*CacheEntry
	lruList  *list.List
	inflight map[string]*InflightCall

	// 统计信息（原子操作，无需加锁）
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64
	updates   atomic.Int64
}
