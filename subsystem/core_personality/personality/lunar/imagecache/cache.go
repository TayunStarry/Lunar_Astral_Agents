package imagecache

import (
	"fmt"
	"sort"
	"time"
)

// FormatTag 将注册序号格式化为图片标签（三位补零，如 图片.003）
func FormatTag(seq int64) string {
	return fmt.Sprintf("图片.%03d", seq)
}

// Put 注册图片来源并分配标签（全局单调，QQ 转发与对话者上下文共用同一命名空间）
//
// pinned 表示图片仍在多模态窗口内（钉住防淘汰）；合并转发的图片从不进入多模态窗口，传 false。
// 容量超限时 FIFO 淘汰最老的未钉住条目；钉住条目超限时强制解钉最老者（其图片多半已被上下文裁剪）
func Put(source string, pinned bool) string {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()

	imageCacheSeq++
	tag := FormatTag(imageCacheSeq)
	imageCacheEntries[tag] = &ImageEntry{
		Seq:       imageCacheSeq,
		Tag:       tag,
		Source:    source,
		Pinned:    pinned,
		Timestamp: time.Now().Unix(),
	}
	imageCacheOrder = append(imageCacheOrder, tag)

	// 钉住条目超限：强制解钉最老的钉住条目
	pinnedCount := 0
	for _, t := range imageCacheOrder {
		if entry := imageCacheEntries[t]; entry != nil && entry.Pinned {
			pinnedCount++
		}
	}
	for pinnedCount > imageCacheMaxPinned {
		unpinned := false
		for _, t := range imageCacheOrder {
			if entry := imageCacheEntries[t]; entry != nil && entry.Pinned {
				entry.Pinned = false
				pinnedCount--
				unpinned = true
				break
			}
		}
		if !unpinned {
			break
		}
	}

	// 容量超限：FIFO 淘汰最老的未钉住条目
	for len(imageCacheOrder) > ImageCacheCapacity {
		if !evictOldestUnpinned() {
			break
		}
	}
	return tag
}

// evictOldestUnpinned 淘汰注册顺序最前的未钉住条目，无可淘汰条目时返回 false
func evictOldestUnpinned() bool {
	for i, tag := range imageCacheOrder {
		if entry := imageCacheEntries[tag]; entry != nil && !entry.Pinned {
			delete(imageCacheEntries, tag)
			imageCacheOrder = append(imageCacheOrder[:i], imageCacheOrder[i+1:]...)
			return true
		}
	}
	return false
}

// Get 按标签查询缓存条目（返回副本），不存在时返回 false
func Get(tag string) (ImageEntry, bool) {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()
	entry, ok := imageCacheEntries[tag]
	if !ok {
		return ImageEntry{}, false
	}
	return *entry, true
}

// List 按注册序号倒序返回最近 topK 个条目（topK <= 0 时返回全部）
func List(topK int) []ImageEntry {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()
	entries := make([]ImageEntry, 0, len(imageCacheEntries))
	for _, entry := range imageCacheEntries {
		entries = append(entries, *entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Seq > entries[j].Seq })
	if topK > 0 && len(entries) > topK {
		entries = entries[:topK]
	}
	return entries
}

// SetDesc 写入条目内容描述（降级时由视觉模型生成一次）
func SetDesc(tag, desc string) {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()
	if entry, ok := imageCacheEntries[tag]; ok {
		entry.Desc = desc
	}
}

// Unpin 解除钉住（图片降级为占位标签后调用，恢复其可淘汰资格）
func Unpin(tag string) {
	imageCacheMu.Lock()
	defer imageCacheMu.Unlock()
	if entry, ok := imageCacheEntries[tag]; ok {
		entry.Pinned = false
	}
}
