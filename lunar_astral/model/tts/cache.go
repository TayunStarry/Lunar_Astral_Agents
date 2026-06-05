package tts

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"logger"
	"sync"
	"sync/atomic"
)

const (
	// MaxCacheSize 缓存容量上限
	MaxCacheSize = 5
)

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

// NewTTSCache 创建新的TTS缓存实例
func NewTTSCache(capacity int) *TTSCache {
	return &TTSCache{
		capacity: capacity,
		items:    make(map[string]*CacheEntry),
		lruList:  list.New(),
		inflight: make(map[string]*InflightCall),
	}
}

// ComputeCacheKey 根据请求参数计算SHA256缓存键，确保不同参数组合产生不同的缓存键
func ComputeCacheKey(params map[string]interface{}) string {
	data, _ := json.Marshal(params)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

// Get 获取缓存中的音频数据，LRU命中时将条目移到链表头部
// 返回值：(音频数据base64, 是否命中)
// 性能：读锁保护，操作复杂度O(1)，预计耗时<1μs
func (c *TTSCache) Get(key string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		logger.SubInfo("TTS-Cache", "未命中", "缓存未命中: key=%s", key[:16])
		return "", false
	}

	// 命中后需要移动LRU位置，升级为写锁
	c.mu.Lock()
	// 双重检查，防止在锁升级期间条目被淘汰
	entry, exists = c.items[key]
	if exists {
		c.lruList.MoveToFront(entry.element)
		c.mu.Unlock()
		c.hits.Add(1)
		logger.SubInfo("TTS-Cache", "命中", "缓存命中: key=%s", key[:16])
		return entry.audioData, true
	}
	c.mu.Unlock()

	c.misses.Add(1)
	logger.SubInfo("TTS-Cache", "未命中", "缓存未命中(竞争淘汰): key=%s", key[:16])
	return "", false
}

// Set 设置缓存值，若键已存在则更新，若达容量上限则触发LRU淘汰
func (c *TTSCache) Set(key, audioData string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果键已存在，更新值并移到链表头部
	if entry, exists := c.items[key]; exists {
		entry.audioData = audioData
		c.lruList.MoveToFront(entry.element)
		c.updates.Add(1)
		logger.SubInfo("TTS-Cache", "更新", "缓存更新: key=%s, 当前条目数=%d", key[:16], c.lruList.Len())
		return
	}

	// 达到容量上限时，淘汰最久未使用的条目
	for c.lruList.Len() >= c.capacity {
		c.evictLRU()
	}

	// 添加新条目到链表头部
	entry := &CacheEntry{
		key:       key,
		audioData: audioData,
	}
	entry.element = c.lruList.PushFront(entry)
	c.items[key] = entry
	c.updates.Add(1)
	logger.SubInfo("TTS-Cache", "添加", "缓存添加: key=%s, 当前条目数=%d", key[:16], c.lruList.Len())
}

// evictLRU 淘汰链表尾部（最久未使用）的缓存条目，调用前需持有写锁
func (c *TTSCache) evictLRU() {
	element := c.lruList.Back()
	if element == nil {
		return
	}

	entry := element.Value.(*CacheEntry)
	delete(c.items, entry.key)
	c.lruList.Remove(element)
	c.evictions.Add(1)
	logger.SubInfo("TTS-Cache", "淘汰", "LRU淘汰: key=%s, 当前条目数=%d", entry.key[:16], c.lruList.Len())
}

// GetOrCreateInflight 获取或创建单飞调用，防止缓存击穿
// 返回值：(InflightCall, 是否已存在)
// 若已存在则返回已有的调用，调用者应等待其完成；若不存在则创建新调用
func (c *TTSCache) GetOrCreateInflight(key string) (*InflightCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if call, exists := c.inflight[key]; exists {
		return call, true // 已存在，返回已有的调用
	}

	call := &InflightCall{
		result: make(chan inflightResult, 1),
	}
	c.inflight[key] = call
	return call, false // 新创建
}

// RemoveInflight 移除单飞调用记录，合成完成后调用
func (c *TTSCache) RemoveInflight(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, key)
}

// Complete 完成单飞调用，向等待者发送结果
func (call *InflightCall) Complete(data string, err error) {
	call.result <- inflightResult{data: data, err: err}
}

// Wait 等待单飞调用完成并返回结果
func (call *InflightCall) Wait() (string, error) {
	result := <-call.result
	return result.data, result.err
}

// Stats 返回缓存统计信息
func (c *TTSCache) Stats() map[string]int64 {
	return map[string]int64{
		"hits":      c.hits.Load(),
		"misses":    c.misses.Load(),
		"evictions": c.evictions.Load(),
		"updates":   c.updates.Load(),
		"size":      int64(c.lruList.Len()),
	}
}