// 媒体工具变量与常量定义
package module

// ==== GGUF 值类型常量 ====

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

// SupportedFormats 支持重编码的图片格式
var SupportedFormats = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
}
