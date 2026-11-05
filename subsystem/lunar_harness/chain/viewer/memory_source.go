package viewer

// memory_source.go 观影记忆迁移端口在编排层的适配器。

import (
	"context"

	"LunarSubsystem/LunarHarness/engine/memory"
	"LunarSubsystem/LunarHarness/engine/viewerstore"
)

// viewerMemorySource 迁移时读取的旧记忆库来源。
//
// 用变量而非直接构造：chain 侧测试需要注入计数实现，
// 核验评论/选片读取链路不会触碰统一记忆库。
var viewerMemorySource viewerstore.LegacyMemorySource = legacyMemorySource{}

// legacyMemorySource 把统一记忆库的分页读取与删除适配成迁移端口。
type legacyMemorySource struct{}

// ListText 读取一页文本记忆，字段与迁移对账所需一一对应。
//
// 记忆库不可用时返回错误，让迁移保持「未完成」并在下次启动重试。
func (legacyMemorySource) ListText(offset, limit int) ([]viewerstore.LegacyMemoryEntry, int, error) {
	items, total, err := memory.ListWithError("text", offset, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]viewerstore.LegacyMemoryEntry, 0, len(items))
	for _, item := range items {
		out = append(out, viewerstore.LegacyMemoryEntry{
			ID:        item.ID,
			Content:   item.Content,
			Source:    item.Source,
			Timestamp: item.Timestamp,
		})
	}
	return out, total, nil
}

// DeleteEntry 删除一条已迁移的源条目。
func (legacyMemorySource) DeleteEntry(id string) error {
	return memory.Delete(context.Background(), id)
}
