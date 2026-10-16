package imagecache

import "sync"

// ImageCacheCapacity 缓存容量：超出时 FIFO 淘汰最老的未钉住条目
const ImageCacheCapacity = 80

// imageCacheMaxPinned 钉住条目上限（多模态窗口 20 张 + 冗余 10）：
// 防止上下文消息裁剪先于降级发生时，钉住条目只增不减导致缓存泄漏
const imageCacheMaxPinned = 30

// imageCacheMu 缓存读写锁（napcat 转发协程与 goja 适配器并发访问）
var imageCacheMu sync.Mutex

// imageCacheSeq 标签序号发生器（全局单调，保证 QQ 侧与 goja 侧同一命名空间不冲突）
var imageCacheSeq int64

// imageCacheOrder 注册顺序（FIFO 淘汰依据）
var imageCacheOrder []string

// imageCacheEntries 标签 → 条目
var imageCacheEntries = make(map[string]*ImageEntry)
