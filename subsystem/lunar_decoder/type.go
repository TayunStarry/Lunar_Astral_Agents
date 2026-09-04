// Package lunardecoder 实现文件数据的可逆编解码。
package lunardecoder

import "errors"

// FileData 表示一个待编解码的文件数据单元。
type FileData struct {
	// Name 文件名称，编解码过程中原样保留。
	Name string
	// Data 文件数据；编码后为经变换的文本，解码后还原为原始二进制。
	Data []byte
}

// Group 描述一组字符变换操作。
type Group struct {
	// Op 操作类型，取值 0-9。
	//   0/1：数字部分（0-9）向后/向前循环位移
	//   2/3：字母部分（a-z,A-Z）向后/向前循环位移
	//   4/5：符号部分（+、/、=）向后/向前循环位移
	//   6/7：全字符反向排序表转换 + 循环位移
	//   8/9：全字符集内与下一位索引异或 + 循环位移
	Op int
	// Count 操作次数。
	Count int
}

// Round 表示一轮 16 字符密钥的加密规格。
type Round struct {
	// Key 对齐后的 16 字符密钥，用于计算滑移次数。
	Key string
	// Groups 由该轮密钥派生的 8 组操作。
	Groups [8]Group
}

// 模块级错误定义。
var (
	// ErrNilData 文件数据或数据内容为 nil。
	ErrNilData = errors.New("lunardecoder: file data is nil")
	// ErrInvalidBase64 还原出的字符串不是合法 Base64。
	ErrInvalidBase64 = errors.New("lunardecoder: invalid base64 content")
	// ErrInvalidKey 密钥非法。
	ErrInvalidKey = errors.New("lunardecoder: invalid key groups")
	// ErrInvalidRLE 游程编码数据格式非法。
	ErrInvalidRLE = errors.New("lunardecoder: invalid rle data")
)
