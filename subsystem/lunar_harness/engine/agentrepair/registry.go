package agentrepair

// registry.go 注册与调用入口。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Register 注册修复处理器；同 ID 后注册的覆盖先注册的。
func Register(handler Handler) {
	if strings.TrimSpace(handler.ID) == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if _, exists := handlers[handler.ID]; !exists {
		order = append(order, handler.ID)
	}
	handlers[handler.ID] = handler
}

// List 返回当前所有可修复目标（按注册顺序）。
func List() []Info {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Info, 0, len(order))
	for _, id := range order {
		handler, ok := handlers[id]
		if !ok {
			continue
		}
		out = append(out, Info{
			ID:          handler.ID,
			Label:       handler.Label,
			Description: handler.Description,
			Actions:     append([]string(nil), handler.Actions...),
		})
	}
	return out
}

// Inspect 读取指定目标的上下文；limit<=0 表示全部。
func Inspect(id string, limit int) (string, error) {
	handler, ok := lookup(id)
	if !ok {
		return "", fmt.Errorf("没有可读取上下文的目标：%s", id)
	}
	value, err := handler.Inspect(limit)
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Repair 对指定目标执行修复动作。
func Repair(id, action string, params map[string]any) (string, error) {
	handler, ok := lookup(id)
	if !ok {
		return "", fmt.Errorf("没有可修复的目标：%s", id)
	}
	if strings.TrimSpace(action) == "" {
		return "", fmt.Errorf("请指定修复动作（%s）", strings.Join(handler.Actions, " / "))
	}
	if !contains(handler.Actions, action) {
		return "", fmt.Errorf("目标 %s 不支持动作 %s（可用：%s）", id, action, strings.Join(handler.Actions, " / "))
	}
	return handler.Repair(action, params)
}

// lookup 查找处理器。
func lookup(id string) (Handler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	handler, ok := handlers[strings.TrimSpace(id)]
	return handler, ok
}

// contains 判断字符串是否在切片中。
func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
