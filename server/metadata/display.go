package metadata

import (
	"fmt"     // 导入格式化输出包，用于打印日志和调试信息
	"log"     // 导入日志包，用于记录运行时信息
	"reflect" // 导入反射包，用于运行时类型信息和操作
	"strings" // 导入字符串操作包，用于处理字符串
)

// DisplayAllMetadata 将传入的元数据映射表格式化打印输出到控制台。
// 它会遍历元数据中的每个键值对，打印键、值的类型和值本身。
//
// 参数:
//   - metadata: 元数据键值对的映射表（key 是字符串，value 是任意类型）
func DisplayAllMetadata(modelName string, metadata map[string]any) {
	// 打印分隔符，用于分隔不同的元数据输出
	log.Printf("%s", strings.Repeat("-=", 28))
	// 打印模型名称，用于标识当前输出的元数据来源
	log.Printf("GGUF元数据 -> 模型[ %s ]", modelName)
	// 遍历元数据映射表中的所有键值对
	for key, value := range metadata {
		// 获取当前值的类型字符串表示
		typeStr := getTypeString(value)
		// 格式化当前值，设置递归深度为0，便于打印输出
		valueStr := formatValue(value, 0)
		// 按照指定格式打印元数据的键、类型和值
		log.Printf("%-40s %-20s = %s\n", key, typeStr, valueStr)
	}
	// 打印分隔符，用于分隔不同的元数据输出
	log.Printf("%s", strings.Repeat("-=", 28))
}

// getTypeString 获取一个值的类型的字符串表示形式。
//
// 参数:
//   - value: 任意类型的值（interface{} 类型）
//
// 返回值:
//   - string: 对应的类型标识符
func getTypeString(value any) string {
	switch v := value.(type) {
	case uint8:
		return "uint8"
	case int8:
		return "int8"
	case uint16:
		return "uint16"
	case int16:
		return "int16"
	case uint32:
		return "uint32"
	case int32:
		return "int32"
	case float32:
		return "float32"
	case bool:
		return "bool"
	case string:
		return "string"
	case []any:
		if len(v) == 0 {
			return "arr[]"
		}
		elemType := getTypeString(v[0])
		return fmt.Sprintf("arr[%s,%d]", elemType, len(v))
	case uint64:
		return "uint64"
	case int64:
		return "int64"
	case float64:
		return "float64"
	default:
		return fmt.Sprintf("unknown(%s)", reflect.TypeOf(v).String())
	}
}

// formatValue 将给定值格式化为可读字符串，适用于打印输出。
//
// 参数:
//   - value: 需要格式化的任意类型值
//   - depth: 递归深度限制（防止无限递归）
//
// 返回值:
//   - string: 格式化后的字符串表示
func formatValue(value any, depth int) string {
	// 检查递归深度，若超过3层则返回省略号，防止无限递归
	if depth > 3 {
		return "..."
	}
	// 根据值的类型进行不同的格式化处理
	switch v := value.(type) {
	case []any:
		// 若数组为空，直接返回空数组表示
		if len(v) == 0 {
			return "[]"
		}
		// 使用 strings.Builder 高效拼接字符串
		var builder strings.Builder
		// 写入数组开始符号
		builder.WriteString("[")
		// 遍历数组元素
		for i, elem := range v {
			// 若不是第一个元素，写入分隔符
			if i > 0 {
				builder.WriteString(", ")
			}
			// 若元素数量超过4个且当前索引大于等于3，写入省略信息并跳出循环
			if i >= 3 && len(v) > 4 {
				builder.WriteString(fmt.Sprintf("... (%d items)", len(v)))
				break
			}
			// 递归格式化当前元素并写入
			builder.WriteString(formatValue(elem, depth+1))
		}
		// 写入数组结束符号
		builder.WriteString("]")
		// 返回拼接好的字符串
		return builder.String()
	case string:
		// 若字符串长度超过50，截取前50个字符并添加省略号
		if len(v) > 50 {
			return fmt.Sprintf("\"%s...\"", v[:50])
		}
		// 返回带引号的完整字符串
		return fmt.Sprintf("\"%s\"", v)
	case float32, float64:
		// 将浮点数格式化为保留6位小数的字符串
		return fmt.Sprintf("%.6f", v)
	default:
		// 使用默认格式将值转换为字符串
		return fmt.Sprintf("%v", v)
	}
}
