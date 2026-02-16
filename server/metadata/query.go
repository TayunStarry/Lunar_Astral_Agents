package metadata

import (
	"strings"         // 导入字符串操作包，用于处理字符串
)

// FindMetadataByKeySubstring 在元数据映射表中查找所有键名包含指定子字符串的条目。
//
// 参数:
//   - metadata: 元数据键值对的映射表
//   - substring: 用于匹配键名的关键字（子串）
//
// 返回值:
//   - map[string]any: 所有符合条件的键值对组成的新的映射表
func FindMetadataByKeySubstring(metadata map[string]any, substring string) map[string]any {
	// 创建一个新的映射表，用于存储匹配到的键值对
	result := make(map[string]any)
	// 遍历元数据映射表中的所有键值对
	for key, value := range metadata {
		// 检查当前键是否包含指定的子字符串
		if strings.Contains(key, substring) {
			// 若包含，则将该键值对添加到结果映射表中
			result[key] = value
		}
	}
	// 返回包含所有匹配键值对的映射表
	return result
}

// FindFirstMetadataByKeySubstring 在元数据映射表中查找第一个键名包含指定子字符串的条目。
//
// 参数:
//   - metadata: 元数据键值对的映射表
//   - substring: 用于匹配键名的关键字（子串）
//
// 返回值:
//   - any: 第一个符合条件的元数据值；如果未找到返回 nil
func FindFirstMetadataByKeySubstring(metadata map[string]any, substring string) any {
	// 遍历元数据映射表中的所有键值对
	for key, value := range metadata {
		// 检查当前键是否包含指定的子字符串
		if strings.Contains(key, substring) {
			// 若包含，则返回该键对应的值
			return value
		}
	}
	// 若遍历完所有键值对都未找到匹配项，返回 nil
	return nil
}
