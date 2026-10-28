package resource

// util.go 句柄系统的字节级原语：内容哈希与 base64 解码。
//
// 路径规整见 path.go，图片头部解析见 image_probe.go。

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
)

// sha16Reader 流式读取并返回 SHA-256 前 16 位十六进制
func sha16Reader(r io.Reader) string {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// fileSHA16 流式计算文件 SHA-256 并取前 16 位十六进制，同时返回文件大小
func fileSHA16(localPath string) (string, int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	return sha16Reader(f), info.Size(), nil
}

// base64Decode 解码 base64（兼容标准与 URL 安全字母表、含/不含填充）
func base64Decode(payload string) ([]byte, error) {
	payload = trimSpaceASCII(payload)
	if data, err := base64.StdEncoding.DecodeString(payload); err == nil {
		return data, nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(payload); err == nil {
		return data, nil
	}
	if data, err := base64.URLEncoding.DecodeString(payload); err == nil {
		return data, nil
	}
	return base64.RawURLEncoding.DecodeString(payload)
}

// trimSpaceASCII 去除首尾空白（含换行，data URI 常带换行）
func trimSpaceASCII(s string) string {
	start, end := 0, len(s)
	for start < end && isSpaceByte(s[start]) {
		start++
	}
	for end > start && isSpaceByte(s[end-1]) {
		end--
	}
	return s[start:end]
}

// isSpaceByte 判断是否为 ASCII 空白字符
func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
