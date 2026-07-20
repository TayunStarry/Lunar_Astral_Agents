package napcat

import (
	"sync"
	"testing"
)

// resetCache 重置缓存状态
func resetCache() {
	messageCache = &MessageCache{}
}

func TestAddCachedMessage_FIFO(t *testing.T) {
	resetCache()

	// 添加 maxCacheSize+5 条消息
	for i := 0; i < maxCacheSize+5; i++ {
		AddCachedMessage(CachedMessage{
			GroupID:  123,
			UserID:   int64(i),
			Nickname: "test",
			Content:  "msg",
		})
	}

	// 缓存大小应不超过 maxCacheSize
	size := GetCacheSize()
	if size != maxCacheSize {
		t.Errorf("缓存大小 = %d, 期望 %d", size, maxCacheSize)
	}

	// 最老的消息应被淘汰（FIFO）
	messages := GetCachedMessages()
	// 最老保留的消息应为第5条（0-indexed），前5条被淘汰
	if messages[0].UserID != 5 {
		t.Errorf("FIFO淘汰后最早消息 UserID = %d, 期望 5", messages[0].UserID)
	}
}

func TestAddCachedMessage_WithinCapacity(t *testing.T) {
	resetCache()

	for i := 0; i < 10; i++ {
		AddCachedMessage(CachedMessage{
			GroupID:  123,
			UserID:   int64(i),
			Nickname: "test",
			Content:  "msg",
		})
	}

	if GetCacheSize() != 10 {
		t.Errorf("缓存大小 = %d, 期望 10", GetCacheSize())
	}
}

func TestClearCachedMessages(t *testing.T) {
	resetCache()

	AddCachedMessage(CachedMessage{GroupID: 1, UserID: 1, Content: "test"})
	AddCachedMessage(CachedMessage{GroupID: 2, UserID: 2, Content: "test"})

	ClearCachedMessages()

	if GetCacheSize() != 0 {
		t.Errorf("清空后缓存大小 = %d, 期望 0", GetCacheSize())
	}
}

func TestGetCachedMessages_Copy(t *testing.T) {
	resetCache()

	AddCachedMessage(CachedMessage{GroupID: 1, UserID: 1, Content: "original"})

	messages := GetCachedMessages()
	messages[0].Content = "modified"

	// 原始缓存不应被修改
	original := GetCachedMessages()
	if original[0].Content != "original" {
		t.Errorf("GetCachedMessages 返回非副本，修改影响了原始缓存")
	}
}

func TestConcurrentCacheAccess(t *testing.T) {
	resetCache()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			AddCachedMessage(CachedMessage{
				GroupID: int64(idx % 5),
				UserID:  int64(idx),
				Content: "concurrent",
			})
		}(i)
	}
	wg.Wait()

	if GetCacheSize() > maxCacheSize {
		t.Errorf("并发写入后缓存大小 = %d, 超过最大容量 %d", GetCacheSize(), maxCacheSize)
	}
}
