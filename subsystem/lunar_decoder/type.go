// Package lunardecoder 实现文件数据的特殊编码与解码功能。
//
// 编码流程：先对每个文件的二进制数据执行密钥绑定的游程编码，再编码为标准
// Base64（RFC 4648）字符串，随后依据 16 位整数密钥派生出的 8 组操作
// （每组包含操作类型与操作次数）对 Base64 字符串依次执行一系列可逆的字符变换，
// 得到编码后的文件数据。
//
// 解码流程：严格按照编码的逆序恢复，并应用每组操作的逆操作，再对还原的
// Base64 字符串依次执行游程解码，最终得到原始二进制数据，保证编码/解码往返一致。
package lunardecoder

import "errors"

// FileData 表示一个待编码或待解码的文件数据单元。
type FileData struct {
	// Name 文件名称，编解码过程中原样保留。
	Name string
	// Data 文件数据。编码后 Data 为经字符变换后的 Base64 文本字节序列；
	// 解码后 Data 还原为原始二进制内容。
	Data []byte
}

// Group 描述一组字符变换操作。
type Group struct {
	// Op 操作类型，取值范围 0-9。
	//   0/1：数字部分（0-9）向后/向前循环位移
	//   2/3：字母部分（a-z,A-Z）向后/向前循环位移
	//   4/5：符号部分（+、/、=）向后/向前循环位移
	//   6/7：全字符反向排序表转换 + 向后/向前循环位移
	//   8/9：全字符集内与下一位字符索引异或 + 向后/向前循环位移
	Op int
	// Count 操作次数，取值范围 0-9。
	Count int
}

// 模块级错误定义。
var (
	// ErrNilData 当文件数据或数据内容为 nil 时返回。
	ErrNilData = errors.New("lunardecoder: file data is nil")
	// ErrInvalidBase64 当解码时还原出的字符串不是合法 Base64 时返回。
	ErrInvalidBase64 = errors.New("lunardecoder: invalid base64 content")
	// ErrInvalidKey 当密钥派生出的操作小组非法时返回。
	ErrInvalidKey = errors.New("lunardecoder: invalid key groups")
	// ErrInvalidRLE 当游程编码数据格式非法时返回。
	ErrInvalidRLE = errors.New("lunardecoder: invalid rle data")
)
