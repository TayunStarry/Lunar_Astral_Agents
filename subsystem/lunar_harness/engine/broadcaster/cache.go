package broadcaster

// cache.go 统一合成缓存：LRU + 单飞，声明的类型在 type.go、实例在 variable.go。
//
// 缓存是 Broadcaster 的标准内置能力，不再下放到单个引擎包：
// 任何引擎的成功合成结果都以「引擎 + 统一请求参数」为键写入同一张 LRU 表，
// 命中直接返回音频、不触碰引擎槽位；并发同一请求由单飞调用合并为一次合成。

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// newSynthCache 创建合成缓存实例
func newSynthCache(capacity int) *SynthCache {
	if capacity <= 0 {
		capacity = 1
	}
	return &SynthCache{
		capacity: capacity,
		items:    make(map[string]*synthCacheEntry),
		lruList:  list.New(),
		inflight: make(map[string]*synthInflightCall),
	}
}

// buildSynthCacheKey 由引擎与统一请求参数计算缓存键（sha256）
func buildSynthCacheKey(engine string, req TTSRequest) string {
	raw, _ := json.Marshal(map[string]any{
		"engine":             engine,
		"text":               req.Text,
		"ref_audio":          req.RefAudio,
		"ref_text":           req.RefText,
		"voice":              req.Voice,
		"mix":                req.Mix,
		"speed":              req.Speed,
		"lang":               req.Lang,
		"instruct":           req.Instruct,
		"seed":               req.Seed,
		"disable_postproc":   req.DisablePostproc,
		"language_id":        req.LanguageID,
		"temperature":        req.Temperature,
		"top_k":              req.TopK,
		"top_p":              req.TopP,
		"max_tokens":         req.MaxTokens,
		"repetition_penalty": req.RepetitionPenalty,
		"threads":            req.Threads,
	})
	hash := sha256.Sum256(raw)
	return fmt.Sprintf("%x", hash)
}

// Get 查询缓存；命中时把条目移到 LRU 头部
func (c *SynthCache) Get(key string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.items[key]
	c.mu.RUnlock()
	if !exists {
		c.misses.Add(1)
		logger.SubInfo(logModule, logModuleCache, "未命中: key=%s", truncateCacheKey(key))
		return "", false
	}

	c.mu.Lock()
	// 锁升级期间条目可能已被淘汰，需要二次确认
	entry, exists = c.items[key]
	if exists {
		c.lruList.MoveToFront(entry.element)
		c.mu.Unlock()
		c.hits.Add(1)
		logger.SubInfo(logModule, logModuleCache, "命中: key=%s", truncateCacheKey(key))
		return entry.audio, true
	}
	c.mu.Unlock()

	c.misses.Add(1)
	logger.SubInfo(logModule, logModuleCache, "未命中(竞争淘汰): key=%s", truncateCacheKey(key))
	return "", false
}

// Set 写入缓存；按键已存在则更新并移到头部，容量满时淘汰最久未用条目
func (c *SynthCache) Set(key, audio string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.items[key]; exists {
		entry.audio = audio
		c.lruList.MoveToFront(entry.element)
		logger.SubInfo(logModule, logModuleCache, "更新: key=%s, 条目数=%d", truncateCacheKey(key), c.lruList.Len())
		return
	}
	for c.lruList.Len() >= c.capacity {
		c.evictLRU()
	}
	entry := &synthCacheEntry{key: key, audio: audio}
	entry.element = c.lruList.PushFront(entry)
	c.items[key] = entry
	logger.SubInfo(logModule, logModuleCache, "写入: key=%s, 条目数=%d", truncateCacheKey(key), c.lruList.Len())
}

// evictLRU 淘汰链表尾部条目（调用前需持有写锁）
func (c *SynthCache) evictLRU() {
	element := c.lruList.Back()
	if element == nil {
		return
	}
	entry, _ := element.Value.(*synthCacheEntry)
	if entry != nil {
		delete(c.items, entry.key)
	}
	c.lruList.Remove(element)
	c.evictions.Add(1)
	logger.SubInfo(logModule, logModuleCache, "LRU 淘汰: key=%s, 条目数=%d", truncateCacheKey(entry.key), c.lruList.Len())
}

// GetOrCreateInflight 获取或创建单飞调用；返回 (调用, 是否已存在)
func (c *SynthCache) GetOrCreateInflight(key string) (*synthInflightCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if call, exists := c.inflight[key]; exists {
		return call, true
	}
	call := &synthInflightCall{done: make(chan struct{})}
	c.inflight[key] = call
	return call, false
}

// RemoveInflight 移除单飞调用记录
func (c *SynthCache) RemoveInflight(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, key)
}

// Complete 完成单飞调用并向所有等待者广播结果
func (call *synthInflightCall) Complete(audio string, err error) {
	call.mu.Lock()
	defer call.mu.Unlock()
	if call.completed {
		return
	}
	call.result = audio
	call.err = err
	call.completed = true
	close(call.done)
}

// Wait 等待单飞调用完成
func (call *synthInflightCall) Wait() (string, error) {
	<-call.done
	return call.result, call.err
}

// Stats 缓存统计
func (c *SynthCache) Stats() map[string]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]int64{
		"hits":      c.hits.Load(),
		"misses":    c.misses.Load(),
		"evictions": c.evictions.Load(),
		"size":      int64(c.lruList.Len()),
	}
}

// truncateCacheKey 截断缓存键用于日志展示
func truncateCacheKey(key string) string {
	if len(key) > 16 {
		return key[:16]
	}
	return key
}
