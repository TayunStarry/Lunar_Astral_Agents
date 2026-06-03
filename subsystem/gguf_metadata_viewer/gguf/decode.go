package gguf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// GGUF值类型常量，用于标识GGUF文件中不同数据类型的值
const (
	GGUFTypeUint8   uint32 = 0
	GGUFTypeInt8    uint32 = 1
	GGUFTypeUint16  uint32 = 2
	GGUFTypeInt16   uint32 = 3
	GGUFTypeUint32  uint32 = 4
	GGUFTypeInt32   uint32 = 5
	GGUFTypeFloat32 uint32 = 6
	GGUFTypeBool    uint32 = 7
	GGUFTypeString  uint32 = 8
	GGUFTypeArray   uint32 = 9
	GGUFTypeUint64  uint32 = 10
	GGUFTypeInt64   uint32 = 11
	GGUFTypeFloat64 uint32 = 12
)

// ParseMetadataFromReader 从任意 io.Reader 中解析 GGUF 格式的元数据。
// 仅读取 header 部分的元数据键值对，不会读取后续的 tensor 数据。
// 适用于网络流、管道等场景，无需将完整的 GGUF 大文件写入磁盘。
func ParseMetadataFromReader(reader io.Reader) (map[string]any, error) {
	// 读取并验证魔数
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

	// 读取版本号
	var version uint32
	if err := binary.Read(reader, endian, &version); err != nil {
		return nil, fmt.Errorf("读取GGUF版本号失败: %w", err)
	}

	// 读取张量和元数据数量
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

	_ = tensorCount // tensor count is informational only

	// 仅解析元数据键值对（跳过后续 tensor info 和 tensor data）
	metadata := make(map[string]any)
	for i := uint64(0); i < metadataCount; i++ {
		key, err := readString(reader, endian)
		if err != nil {
			return nil, fmt.Errorf("读取元数据键失败: %w", err)
		}

		var valueType uint32
		if errRead := binary.Read(reader, endian, &valueType); errRead != nil {
			return nil, fmt.Errorf("读取元数据值类型失败: %w", errRead)
		}

		value, err := readValue(reader, valueType, endian, version)
		if err != nil {
			return nil, fmt.Errorf("读取元数据值失败(key=%s): %w", key, err)
		}
		metadata[key] = value
	}

	return metadata, nil
}

// ParseMetadata 解析指定路径的 GGUF 文件，提取所有元数据键值对。
// 内部调用 ParseMetadataFromReader，仅读取文件 header。
func ParseMetadata(filePath string) (map[string]any, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件 %s: %w", filePath, err)
	}
	defer file.Close()

	return ParseMetadataFromReader(file)
}

// readString 从 io.Reader 中读取一个 GGUF 格式的字符串。
func readString(reader io.Reader, endian binary.ByteOrder) (string, error) {
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

// readValue 根据指定的数据类型从 Reader 中读取一个值。
func readValue(reader io.Reader, valueType uint32, endian binary.ByteOrder, version uint32) (any, error) {
	switch valueType {
	case GGUFTypeUint8:
		var val uint8
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeInt8:
		var val int8
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeUint16:
		var val uint16
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeInt16:
		var val int16
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeUint32:
		var val uint32
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeInt32:
		var val int32
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeFloat32:
		var val float32
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeBool:
		var b uint8
		if err := binary.Read(reader, endian, &b); err != nil {
			return nil, err
		}
		return b == 1, nil
	case GGUFTypeString:
		return readString(reader, endian)
	case GGUFTypeArray:
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
			elem, err := readValue(reader, elemType, endian, version)
			if err != nil {
				return nil, err
			}
			arr[i] = elem
		}
		return arr, nil
	case GGUFTypeUint64:
		var val uint64
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeInt64:
		var val int64
		err := binary.Read(reader, endian, &val)
		return val, err
	case GGUFTypeFloat64:
		var val float64
		err := binary.Read(reader, endian, &val)
		return val, err
	default:
		return nil, fmt.Errorf("不支持的值类型: %d", valueType)
	}
}