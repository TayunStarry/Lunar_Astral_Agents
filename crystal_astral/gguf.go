package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// GGUF 值类型常量
const (
	ggufTypeUint8   uint32 = 0
	ggufTypeInt8    uint32 = 1
	ggufTypeUint16  uint32 = 2
	ggufTypeInt16   uint32 = 3
	ggufTypeUint32  uint32 = 4
	ggufTypeInt32   uint32 = 5
	ggufTypeFloat32 uint32 = 6
	ggufTypeBool    uint32 = 7
	ggufTypeString  uint32 = 8
	ggufTypeArray   uint32 = 9
	ggufTypeUint64  uint32 = 10
	ggufTypeInt64   uint32 = 11
	ggufTypeFloat64 uint32 = 12
)

// parseGGUFMetadata 从 io.Reader 解析 GGUF 格式的元数据
func parseGGUFMetadata(reader io.Reader) (map[string]any, error) {
	magicBytes := make([]byte, 4)
	if _, err := io.ReadFull(reader, magicBytes); err != nil {
		return nil, fmt.Errorf("读取文件魔数失败: %w", err)
	}

	var endian binary.ByteOrder
	magicLE := binary.LittleEndian.Uint32(magicBytes)
	magicBE := binary.BigEndian.Uint32(magicBytes)

	switch {
	case magicLE == 0x46554747:
		endian = binary.LittleEndian
	case magicBE == 0x47554746:
		endian = binary.BigEndian
	default:
		return nil, errors.New("无效的GGUF文件魔数，请确认文件格式正确")
	}

	var version uint32
	if err := binary.Read(reader, endian, &version); err != nil {
		return nil, fmt.Errorf("读取GGUF版本号失败: %w", err)
	}

	var tensorCount, metadataCount uint64
	if version == 1 {
		var v1TensorCount, v1MetadataCount uint32
		if err := binary.Read(reader, endian, &v1TensorCount); err != nil {
			return nil, err
		}
		if err := binary.Read(reader, endian, &v1MetadataCount); err != nil {
			return nil, err
		}
		tensorCount = uint64(v1TensorCount)
		metadataCount = uint64(v1MetadataCount)
	} else if version >= 2 {
		if err := binary.Read(reader, endian, &tensorCount); err != nil {
			return nil, err
		}
		if err := binary.Read(reader, endian, &metadataCount); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("不支持的GGUF版本: %d", version)
	}

	_ = tensorCount

	metadata := make(map[string]any)
	for i := uint64(0); i < metadataCount; i++ {
		key, err := ggufReadString(reader, endian)
		if err != nil {
			return nil, fmt.Errorf("读取元数据键失败: %w", err)
		}

		var valueType uint32
		if errRead := binary.Read(reader, endian, &valueType); errRead != nil {
			return nil, fmt.Errorf("读取元数据值类型失败: %w", errRead)
		}

		value, err := ggufReadValue(reader, valueType, endian, version)
		if err != nil {
			return nil, fmt.Errorf("读取元数据值失败(key=%s): %w", key, err)
		}
		metadata[key] = value
	}

	return metadata, nil
}

// parseGGUFFile 解析指定路径的 GGUF 文件
func parseGGUFFile(filePath string) (map[string]any, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件 %s: %w", filePath, err)
	}
	defer file.Close()

	return parseGGUFMetadata(file)
}

// ggufReadString 读取 GGUF 格式的字符串
func ggufReadString(reader io.Reader, endian binary.ByteOrder) (string, error) {
	var length uint64
	if err := binary.Read(reader, endian, &length); err != nil {
		return "", err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data), nil
}

// ggufReadValue 根据类型读取 GGUF 值
func ggufReadValue(reader io.Reader, valueType uint32, endian binary.ByteOrder, version uint32) (any, error) {
	switch valueType {
	case ggufTypeUint8:
		var val uint8
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeInt8:
		var val int8
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeUint16:
		var val uint16
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeInt16:
		var val int16
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeUint32:
		var val uint32
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeInt32:
		var val int32
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeFloat32:
		var val float32
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeBool:
		var b uint8
		if err := binary.Read(reader, endian, &b); err != nil {
			return nil, err
		}
		return b == 1, nil
	case ggufTypeString:
		return ggufReadString(reader, endian)
	case ggufTypeArray:
		var elemType uint32
		if err := binary.Read(reader, endian, &elemType); err != nil {
			return nil, err
		}
		var length uint64
		if err := binary.Read(reader, endian, &length); err != nil {
			return nil, err
		}
		arr := make([]any, length)
		for i := uint64(0); i < length; i++ {
			elem, err := ggufReadValue(reader, elemType, endian, version)
			if err != nil {
				return nil, err
			}
			arr[i] = elem
		}
		return arr, nil
	case ggufTypeUint64:
		var val uint64
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeInt64:
		var val int64
		err := binary.Read(reader, endian, &val)
		return val, err
	case ggufTypeFloat64:
		var val float64
		err := binary.Read(reader, endian, &val)
		return val, err
	default:
		return nil, fmt.Errorf("不支持的值类型: %d", valueType)
	}
}

// formatGGUFValue 将 GGUF 元数据值格式化为字符串
func formatGGUFValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%.6f", v)
	case []any:
		parts := make([]string, 0, len(v))
		for i, elem := range v {
			if i > 10 {
				parts = append(parts, fmt.Sprintf("... (%d items)", len(v)))
				break
			}
			parts = append(parts, formatGGUFValue(elem))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// extractGGUFSummary 从元数据中提取模型摘要
func extractGGUFSummary(metadata map[string]any, filename string) map[string]string {
	summary := make(map[string]string)

	if name, ok := getGGUFString(metadata, "general.name"); ok {
		summary["Model Name"] = name
	} else {
		summary["Model Name"] = filename
	}

	if arch, ok := getGGUFString(metadata, "general.architecture"); ok {
		summary["Architecture"] = arch
	}

	if fileType, ok := getGGUFString(metadata, "general.file_type"); ok {
		summary["Quantization"] = fileType
	}

	if qv := getGGUFAny(metadata, "general.quantization_version"); qv != nil {
		summary["Quant Version"] = formatGGUFValue(qv)
	}

	// 上下文长度
	if ctxLen := getGGUFAny(metadata, "llama.context_length"); ctxLen != nil {
		summary["Context Length"] = formatGGUFValue(ctxLen)
	} else if ctxLen = getGGUFAny(metadata, "qwen2.context_length"); ctxLen != nil {
		summary["Context Length"] = formatGGUFValue(ctxLen)
	}

	// 嵌入维度
	if embLen := getGGUFAny(metadata, "llama.embedding_length"); embLen != nil {
		summary["Embedding Dim"] = formatGGUFValue(embLen)
	} else if embLen = getGGUFAny(metadata, "qwen2.embedding_length"); embLen != nil {
		summary["Embedding Dim"] = formatGGUFValue(embLen)
	}

	// 层数
	if blockCount := getGGUFAny(metadata, "llama.block_count"); blockCount != nil {
		summary["Block Count"] = formatGGUFValue(blockCount)
	} else if blockCount = getGGUFAny(metadata, "qwen2.block_count"); blockCount != nil {
		summary["Block Count"] = formatGGUFValue(blockCount)
	}

	// 注意力头数
	if headCount := getGGUFAny(metadata, "llama.attention.head_count"); headCount != nil {
		summary["Attention Heads"] = formatGGUFValue(headCount)
	} else if headCount = getGGUFAny(metadata, "qwen2.attention.head_count"); headCount != nil {
		summary["Attention Heads"] = formatGGUFValue(headCount)
	}

	// KV 头数
	if headCountKV := getGGUFAny(metadata, "llama.attention.head_count_kv"); headCountKV != nil {
		summary["KV Heads"] = formatGGUFValue(headCountKV)
	} else if headCountKV = getGGUFAny(metadata, "qwen2.attention.head_count_kv"); headCountKV != nil {
		summary["KV Heads"] = formatGGUFValue(headCountKV)
	}

	// FFN 维度
	if ffnLen := getGGUFAny(metadata, "llama.feed_forward_length"); ffnLen != nil {
		summary["FFN Dim"] = formatGGUFValue(ffnLen)
	} else if ffnLen = getGGUFAny(metadata, "qwen2.feed_forward_length"); ffnLen != nil {
		summary["FFN Dim"] = formatGGUFValue(ffnLen)
	}

	// 词表大小
	if vocabSize := getGGUFAny(metadata, "tokenizer.ggml.token_count"); vocabSize != nil {
		summary["Vocab Size"] = formatGGUFValue(vocabSize)
	}

	return summary
}

func getGGUFString(metadata map[string]any, key string) (string, bool) {
	if val, ok := metadata[key]; ok {
		if s, ok := val.(string); ok {
			return s, ok
		}
	}
	return "", false
}

func getGGUFAny(metadata map[string]any, key string) any {
	if val, ok := metadata[key]; ok {
		return val
	}
	return nil
}