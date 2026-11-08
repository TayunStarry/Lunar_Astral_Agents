package memory

// tagging.go 标签生成的装配入口。
// 模型调用、重试与输出解析由 agents/MemoryTagger 智能体负责，
// 记忆库只负责把业务字段整理成调用参数。

import (
	"context"
	"fmt"
)

// generateTags 调用标签智能体为文本或单图生成中文标签。
func (d *Store) generateTags(ctx context.Context, content string, isImage bool, orientation string, custom string) ([]string, error) {
	if TagGenerator == nil {
		return nil, fmt.Errorf("标签生成智能体未装配")
	}
	return TagGenerator(ctx, content, isImage, orientation, custom, nil)
}

// generateTagsFromMedia 调用标签智能体为动态图采样帧序列生成标签。
func (d *Store) generateTagsFromMedia(ctx context.Context, mediaRefs []string, orientation string, custom string) ([]string, error) {
	if TagGenerator == nil {
		return nil, fmt.Errorf("标签生成智能体未装配")
	}
	return TagGenerator(ctx, "", true, orientation, custom, mediaRefs)
}
