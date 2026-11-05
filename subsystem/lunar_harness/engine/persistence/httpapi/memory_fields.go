package httpapi

// memory_fields.go 请求字段读取

func memoryQueryModeText(forceGlobal bool) string {
	if forceGlobal {
		return "全局检索"
	}
	return "快速检索"
}

// getStringField 从 map[string]any 安全获取字符串字段
func getStringField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getIntField 从 map[string]any 安全获取整数字段
func getIntField(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return 0
}

// getBoolField 从 map[string]any 安全获取布尔字段
func getBoolField(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
