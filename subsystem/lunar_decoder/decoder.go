package lunardecoder

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// EncodeFiles 对文件数据数组执行编码。
//
// 对每个文件的二进制数据执行：
//  1. 依据 key 派生的控制字节执行密钥绑定的游程编码；
//  2. 转换为标准 Base64（RFC 4648）字符串；
//  3. 依据 key 派生的 8 组操作对 Base64 字符串执行字符变换；
//  4. 变换后的 Base64 文本作为该文件的编码数据（Data）。
//
// 返回的 [FileData] 数组与输入一一对应，Name 原样保留。若 files 或任一文件
// Data 为 nil，或密钥非法，则返回对应错误。使用一致的 key 调用 [DecodeFiles]
// 可还原原始数据。
func EncodeFiles(files []FileData, key int16) ([]FileData, error) {
	groups, err := KeyGroups(key)
	if err != nil {
		return nil, err
	}
	return encodeWithGroups(files, groups)
}

// EncodeFilesWithKeyString 使用数字字符串密钥（16 位整数密钥的字符形式）编码。
//
// keyStr 为十进制数字字符组成的密钥字符串，长度不限于 16 位：
//   - 不足 16 位：右侧补 '0' 至 16 位；
//   - 超过 16 位：按每段 16 位从左拆分，最后一段不足 16 位时右侧补 '0'，
//     并对各段逐位不进位加法（0-9 取模）合并为单一 16 位密钥，再拆分为 8 组操作。
//
// 例如 34 位密钥会被拆成两段 16 位 + 一段补零后相加合并。
func EncodeFilesWithKeyString(files []FileData, keyStr string) ([]FileData, error) {
	groups, err := keyStringGroups(keyStr)
	if err != nil {
		return nil, err
	}
	return encodeWithGroups(files, groups)
}

func encodeWithGroups(files []FileData, groups [8]Group) ([]FileData, error) {
	if files == nil {
		return nil, ErrNilData
	}
	// 由密钥派生游程编码控制字节，使编码格式与密钥绑定（额外混淆层）。
	seed := rleSeed(groups)
	out := make([]FileData, len(files))
	for i, f := range files {
		if f.Data == nil {
			return nil, ErrNilData
		}
		// 二进制数据 -> 密钥绑定的游程编码 -> Base64 文本 -> 字符变换后的编码数据。
		text := base64.StdEncoding.EncodeToString(rleEncode(f.Data, seed))
		transformed := ApplyTransform([]byte(text), groups, false)
		if transformed == nil {
			transformed = []byte{}
		}
		out[i] = FileData{Name: f.Name, Data: transformed}
	}
	return out, nil
}

// DecodeFiles 对编码后的文件数据数组执行解码。
//
// 对每个编码文件的 Data 执行：
//  1. 依据 key 派生的 8 组操作逆序恢复原始 Base64 字符串；
//  2. 将 Base64 字符串解码为二进制数据；
//  3. 依据 key 派生的控制字节执行密钥绑定的游程解码，还原原始内容。
//
// 必须使用与 [EncodeFiles] 相同的 key。若 files 或任一文件 Data 为 nil、
// 密钥非法，或还原出的内容不是合法 Base64 / 游程编码，则返回对应错误。
func DecodeFiles(files []FileData, key int16) ([]FileData, error) {
	groups, err := KeyGroups(key)
	if err != nil {
		return nil, err
	}
	return decodeWithGroups(files, groups)
}

// DecodeFilesWithKeyString 使用数字字符串密钥解码，密钥处理规则同 [EncodeFilesWithKeyString]。
func DecodeFilesWithKeyString(files []FileData, keyStr string) ([]FileData, error) {
	groups, err := keyStringGroups(keyStr)
	if err != nil {
		return nil, err
	}
	return decodeWithGroups(files, groups)
}

func decodeWithGroups(files []FileData, groups [8]Group) ([]FileData, error) {
	if files == nil {
		return nil, ErrNilData
	}
	// 与编码一致，由密钥派生游程编码控制字节。
	seed := rleSeed(groups)
	out := make([]FileData, len(files))
	for i, f := range files {
		if f.Data == nil {
			return nil, ErrNilData
		}
		// 字符逆变换 -> 原始 Base64 文本 -> 游程解码 -> 二进制数据。
		recovered := ApplyTransform(f.Data, groups, true)
		rle, err := base64.StdEncoding.DecodeString(string(recovered))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidBase64, err)
		}
		raw, err := rleDecode(rle, seed)
		if err != nil {
			return nil, err
		}
		out[i] = FileData{Name: f.Name, Data: raw}
	}
	return out, nil
}

// keyStringGroups 将数字字符串密钥转换为操作小组。
func keyStringGroups(keyStr string) ([8]Group, error) {
	keyStr = strings.TrimSpace(keyStr)
	if keyStr == "" {
		return [8]Group{}, ErrInvalidKey
	}
	for _, r := range keyStr {
		if r < '0' || r > '9' {
			return [8]Group{}, ErrInvalidKey
		}
	}
	return combineKeySegments(keyStr)
}