package lunardecoder

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

// EncodeFiles 对文件数据数组执行编码。
// 每轮流程：RLE -> 扩散 -> Base64 -> 字符变换 -> 滑移。
func EncodeFiles(files []FileData, key int16) ([]FileData, error) {
	rounds, err := keySpec(strconv.FormatUint(uint64(uint16(key)), 10))
	if err != nil {
		return nil, err
	}
	return encodeWithRounds(files, rounds)
}

// EncodeFilesWithKeyString 使用密钥字符串编码。
func EncodeFilesWithKeyString(files []FileData, keyStr string) ([]FileData, error) {
	rounds, err := keySpec(keyStr)
	if err != nil {
		return nil, err
	}
	return encodeWithRounds(files, rounds)
}

// encodeWithRounds 对每个文件依次应用每一轮加密，后一轮以前一轮结果为输入。
func encodeWithRounds(files []FileData, rounds []Round) ([]FileData, error) {
	if files == nil {
		return nil, ErrNilData
	}
	out := make([]FileData, len(files))
	for i, f := range files {
		if f.Data == nil {
			return nil, ErrNilData
		}
		data := f.Data
		for _, r := range rounds {
			data = encodeRound(data, r)
		}
		out[i] = FileData{Name: f.Name, Data: data}
	}
	return out, nil
}

// encodeRound 单轮加密。
func encodeRound(data []byte, r Round) []byte {
	diffused := diffuseRaw(rleEncode(data, rleSeed(r.Groups)), r.Key, diffuseRounds, false)
	text := base64.StdEncoding.EncodeToString(diffused)
	spec := SlideSpecFromGroups(r.Groups)
	transformed := ApplyTransform([]byte(text), r.Groups, false)
	transformed = SlideRotate(transformed, spec.Direction, SlideAmount(r.Key, len(transformed)))
	if transformed == nil {
		transformed = []byte{}
	}
	return transformed
}

// DecodeFiles 对编码后的文件数据数组执行解码。
func DecodeFiles(files []FileData, key int16) ([]FileData, error) {
	rounds, err := keySpec(strconv.FormatUint(uint64(uint16(key)), 10))
	if err != nil {
		return nil, err
	}
	return decodeWithRounds(files, rounds)
}

// DecodeFilesWithKeyString 使用密钥字符串解码。
func DecodeFilesWithKeyString(files []FileData, keyStr string) ([]FileData, error) {
	rounds, err := keySpec(keyStr)
	if err != nil {
		return nil, err
	}
	return decodeWithRounds(files, rounds)
}

// decodeWithRounds 对每个文件按加密轮次逆序逐轮还原。
func decodeWithRounds(files []FileData, rounds []Round) ([]FileData, error) {
	if files == nil {
		return nil, ErrNilData
	}
	out := make([]FileData, len(files))
	for i, f := range files {
		if f.Data == nil {
			return nil, ErrNilData
		}
		data := f.Data
		for j := len(rounds) - 1; j >= 0; j-- {
			var err error
			data, err = decodeRound(data, rounds[j])
			if err != nil {
				return nil, err
			}
		}
		out[i] = FileData{Name: f.Name, Data: data}
	}
	return out, nil
}

// decodeRound 单轮解码（编码逆序）。
func decodeRound(data []byte, r Round) ([]byte, error) {
	spec := SlideSpecFromGroups(r.Groups)
	recovered := SlideRotate(data, ReverseDirection(spec.Direction), SlideAmount(r.Key, len(data)))
	recovered = ApplyTransform(recovered, r.Groups, true)
	rle, err := base64.StdEncoding.DecodeString(string(recovered))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBase64, err)
	}
	return rleDecode(diffuseRaw(rle, r.Key, diffuseRounds, true), rleSeed(r.Groups))
}
