package agentdump

// registry.go 注册、列举与导出。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Register 注册一个导出目标（重复 ID 以最后一次为准，顺序保持首次注册位置）。
func Register(provider Provider) {
	provider.ID = strings.TrimSpace(provider.ID)
	if provider.ID == "" || provider.Export == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := providers[provider.ID]; !exists {
		order = append(order, provider.ID)
	}
	providers[provider.ID] = provider
}

// List 返回全部已注册目标（按注册顺序）。
func List() []Provider {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Provider, 0, len(order))
	for _, id := range order {
		if provider, ok := providers[id]; ok {
			out = append(out, provider)
		}
	}
	return out
}

// IDs 返回全部导出标识（便于测试与文档）。
func IDs() []string {
	providers := List()
	ids := make([]string, 0, len(providers))
	for _, provider := range providers {
		ids = append(ids, provider.ID)
	}
	sort.Strings(ids)
	return ids
}

// Export 导出指定目标并返回 JSON 文本。
func Export(id string, includeImages bool) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "core"
	}
	mu.RLock()
	provider, ok := providers[id]
	mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("未知导出目标: %s（可用：%s）", id, strings.Join(IDs(), " / "))
	}
	payload, err := provider.Export(includeImages)
	if err != nil {
		return "", err
	}
	if payload == nil {
		return "", fmt.Errorf("导出目标 %s 当前没有可导出的内容", id)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return "", fmt.Errorf("序列化导出内容失败: %w", err)
	}
	return buf.String(), nil
}
