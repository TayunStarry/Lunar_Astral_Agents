package metadata

import (
	"encoding/binary" // 导入二进制编码包，用于处理二进制数据的读写
	"errors"          // 导入错误处理包，用于返回和处理错误信息
	"fmt"             // 导入格式化输出包，用于打印日志和调试信息
	"io"              // 导入IO包，用于文件读写操作
	"os"              // 导入操作系统包，用于文件操作和系统调用
)

// GGUF值类型常量，用于标识GGUF文件中不同数据类型的值
const (
	GGUFTypeUint8   uint32 = 0  // 无符号8位整数
	GGUFTypeInt8    uint32 = 1  // 有符号8位整数
	GGUFTypeUint16  uint32 = 2  // 无符号16位整数
	GGUFTypeInt16   uint32 = 3  // 有符号16位整数
	GGUFTypeUint32  uint32 = 4  // 无符号32位整数
	GGUFTypeInt32   uint32 = 5  // 有符号32位整数
	GGUFTypeFloat32 uint32 = 6  // 32位浮点数
	GGUFTypeBool    uint32 = 7  // 布尔值
	GGUFTypeString  uint32 = 8  // 字符串
	GGUFTypeArray   uint32 = 9  // 数组
	GGUFTypeUint64  uint32 = 10 // 无符号64位整数
	GGUFTypeInt64   uint32 = 11 // 有符号64位整数
	GGUFTypeFloat64 uint32 = 12 // 64位浮点数
)

// ParseMetadata 解析一个 GGUF 格式的文件，并从中提取出所有的元数据键值对。
//
// 参数:
//   - filePath: 要解析的 GGUF 文件路径（字符串）
//
// 返回值:
//   - map[string]interface{}: 包含所有元数据键值对的映射表
//   - error: 如果在读取或解析过程中发生错误，则返回该错误信息
func ParseMetadata(filePath string) (map[string]any, error) {
	// 打开GGUF文件
	file, err := os.Open(filePath)
	// 检查文件是否成功打开
	if err != nil {
		return nil, err
	}
	// 确保文件在函数退出时关闭
	defer file.Close()
	// 读取魔数
	magicBytes := make([]byte, 4)
	// 读取文件前4字节，判断是否为GGUF文件
	if _, err := io.ReadFull(file, magicBytes); err != nil {
		return nil, err
	}
	// 声明一个变量用于存储字节序，后续读写文件时会使用该字节序
	var endian binary.ByteOrder
	// 使用小端字节序解析魔数
	magicLE := binary.LittleEndian.Uint32(magicBytes)
	// 使用大端字节序解析魔数
	magicBE := binary.BigEndian.Uint32(magicBytes)
	// 根据解析结果判断文件的字节序
	switch {
	case magicLE == 0x46554747: // "GGUF" 小端
		// 如果小端解析结果匹配，则使用小端字节序
		endian = binary.LittleEndian
	case magicBE == 0x47554746: // "GGUF" 大端
		// 如果大端解析结果匹配，则使用大端字节序
		endian = binary.BigEndian
	default:
		// 如果魔数不匹配，则返回错误，表明文件不是有效的GGUF文件
		return nil, errors.New("GGUF模块端[ERROR] -> 无效的GGUF文件魔数")
	}
	// 读取版本号
	var version uint32
	// 读取文件版本号，用于后续判断文件格式
	if err := binary.Read(file, endian, &version); err != nil {
		return nil, err
	}

	// 声明变量用于存储张量数量和元数据数量，统一使用 uint64 类型
	var tensorCount, metadataCount uint64
	// 根据不同的文件版本，采用不同的方式读取张量数量和元数据数量
	if version == 1 {
		// 版本 1 的文件中，张量数量和元数据数量使用 uint32 类型存储
		var v1TensorCount, v1MetadataCount uint32
		// 从文件中读取版本 1 的张量数量
		if err := binary.Read(file, endian, &v1TensorCount); err != nil {
			return nil, err
		}
		// 从文件中读取版本 1 的元数据数量
		if err := binary.Read(file, endian, &v1MetadataCount); err != nil {
			return nil, err
		}
		// 将版本 1 的 uint32 类型的数量转换为 uint64 类型
		tensorCount = uint64(v1TensorCount)
		metadataCount = uint64(v1MetadataCount)
	} else if version >= 2 {
		// 版本 2 及以上的文件中，张量数量和元数据数量直接使用 uint64 类型存储
		// 从文件中读取张量数量
		if err := binary.Read(file, endian, &tensorCount); err != nil {
			return nil, err
		}
		// 从文件中读取元数据数量
		if err := binary.Read(file, endian, &metadataCount); err != nil {
			return nil, err
		}
	} else {
		// 如果文件版本小于 1，说明是不支持的版本，返回错误信息
		return nil, fmt.Errorf("GGUF模块端[ERROR] -> 不支持的GGUF版本: %d", version)
	}
	// 解析元数据键值对
	metadata := make(map[string]any)
	// 循环读取元数据，循环次数为元数据数量
	for i := uint64(0); i < metadataCount; i++ {
		// 从文件中读取元数据的键
		key, err := readString(file, endian)
		// 若读取键时发生错误，返回错误信息
		if err != nil {
			return nil, err
		}
		// 声明变量用于存储元数据值的类型
		var valueType uint32
		// 从文件中读取元数据值的类型
		if errRead := binary.Read(file, endian, &valueType); errRead != nil {
			// 若读取值类型时发生错误，返回错误信息
			return nil, errRead
		}
		// 根据值类型从文件中读取对应的元数据值
		value, err := readValue(file, valueType, endian, version)
		// 若读取值时发生错误，返回错误信息
		if err != nil {
			return nil, err
		}
		// 将读取到的键值对存入元数据映射中
		metadata[key] = value
	}
	// 返回解析完成的元数据映射，无错误信息
	return metadata, nil
}

// readString 从给定的 io.Reader 中读取一个 GGUF 格式的字符串。
//
// 参数:
//   - reader: 提供二进制数据输入的 Reader 接口实例
//   - endian: 表示字节序的大端或小端格式（binary.ByteOrder）
//
// 返回值:
//   - string: 读取到的字符串内容
//   - error: 如果读取失败则返回错误信息
func readString(reader io.Reader, endian binary.ByteOrder) (string, error) {
	// 从读取器中读取字符串的长度，使用指定的字节序解析为 uint64 类型
	var length uint64
	// 若读取长度时发生错误，返回空字符串和错误信息
	if err := binary.Read(reader, endian, &length); err != nil {
		return "", err
	}
	// 根据读取到的长度创建一个字节切片，用于存储字符串数据
	data := make([]byte, length)
	// 若读取字符串数据时发生错误，返回空字符串和错误信息
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	// 将读取到的字节切片转换为字符串并返回
	return string(data), nil
}

// readValue 根据指定的数据类型从 Reader 中读取一个值。
//
// 参数:
//   - reader: 提供二进制数据输入的 Reader 接口实例
//   - valueType: 表示要读取的数据类型的常量（如 GGUFTypeUint8 等）
//   - endian: 字节序格式（大端或小端）
//   - version: GGUF 文件版本号（用于处理不同版本之间的差异）
//
// 返回值:
//   - any: 读取出的任意类型的数据
//   - error: 如果在解析过程中出错则返回错误信息
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
		return nil, fmt.Errorf("unsupported value type: %d", valueType)
	}
}
