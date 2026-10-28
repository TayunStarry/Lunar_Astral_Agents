package memory

import "fmt"

// =============================================================================
// 全局只读 / 集合管理包装 — 供外部模块调用
// =============================================================================

// Get 全局按 ID 获取条目（跨全部集合）
func Get(id string) (*Entry, error) {
	if globalStore == nil {
		return nil, errStoreMissing()
	}
	return globalStore.Get(id)
}

// List 全局逐条列举条目（跨全部集合）
func List(mediaType string, offset, limit int) ([]Entry, int) {
	if globalStore == nil {
		return nil, 0
	}
	return globalStore.List(mediaType, offset, limit)
}

// Count 全局统计指定类型的条目数
func Count(mediaType string) int {
	if globalStore == nil {
		return 0
	}
	return globalStore.Count(mediaType)
}

// ListBySource 全局按写入来源列举条目（按入库时间倒序；limit<=0 表示不限）
func ListBySource(mediaType, source string, limit int) []Entry {
	if globalStore == nil {
		return nil
	}
	return globalStore.ListBySource(mediaType, source, limit)
}

// Documents 全局获取集合内文档分页
func Documents(collectionName string, offset, limit int) ([]DocumentEntry, int) {
	if globalStore == nil {
		return nil, 0
	}
	return globalStore.Documents(collectionName, offset, limit)
}

// DocCount 全局获取集合文档数
func DocCount(collectionName string) int {
	if globalStore == nil {
		return 0
	}
	return globalStore.DocCount(collectionName)
}

// CollectionCount 全局获取集合数量
func CollectionCount() int {
	if globalStore == nil {
		return 0
	}
	return globalStore.CollectionCount()
}

// Collections 全局列出集合
func Collections() []string {
	if globalStore == nil {
		return nil
	}
	return globalStore.Collections()
}

// CollectionInfo 全局获取集合信息
func CollectionInfo(collectionName string) map[string]interface{} {
	if globalStore == nil {
		return nil
	}
	return globalStore.CollectionInfo(collectionName)
}

// DeleteCollection 全局删除集合
func DeleteCollection(collectionName string) error {
	if globalStore == nil {
		return errStoreMissing()
	}
	return globalStore.DeleteCollection(collectionName)
}

// ClearCollection 全局清空集合
func ClearCollection(collectionName string) error {
	if globalStore == nil {
		return errStoreMissing()
	}
	return globalStore.ClearCollection(collectionName)
}

// Reload 全局重新加载分组（外部直接改写分片文件后刷新内存缓存）
func Reload(collectionName string) error {
	if globalStore == nil {
		return errStoreMissing()
	}
	return globalStore.Reload(collectionName)
}

// HasSyncMismatch 全局检查同步不一致
func HasSyncMismatch(collectionName string) bool {
	if globalStore == nil {
		return false
	}
	return globalStore.HasSyncMismatch(collectionName)
}

// errStoreMissing 全局实例尚未建立时的统一错误
func errStoreMissing() error {
	return fmt.Errorf("全局记忆库未初始化")
}
