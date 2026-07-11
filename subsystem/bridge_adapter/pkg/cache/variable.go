package cache

// 缓存包的全局变量

import (
	"bridge_adapter/pkg/types"
	"sync"
)

var (
	// GroupCaches 按群号分组的消息缓存 groupID -> *GroupCache
	GroupCaches = make(map[int64]*types.GroupCache)
	// GroupSummaries 按群号分组的摘要缓存 groupID -> []SummaryEntry
	GroupSummaries = make(map[int64][]types.SummaryEntry)
	// CacheMutex 保护缓存并发访问的互斥锁
	CacheMutex sync.RWMutex
)
