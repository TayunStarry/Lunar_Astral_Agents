package StarLTP

// ==== encoding 编解码（allow-certificate） ====
// 提供 base64 / url 编解码与 lunar_encoder 加解密桥（复用 subsystem/lunar_decoder，
// 与 permissions.key 的加解密同源）。入参与返回均为字符串（UTF-8 文本）；
// 二进制数据如需传递可自行用 base64 包裹。

import (
	"encoding/base64"
	"fmt"
	"net/url"

	lunardecoder "LunarSubsystem/LunarDecoder"
	"github.com/dop251/goja"
)

// bindEncoding encoding.base64Encode/base64Decode/urlEncode/urlDecode/lunarEncoder/lunarDecoder。
// 契约（Result 统一风格）：可失败方法返回 { success, text?, error? }，不再抛异常。
func bindEncoding(vm *goja.Runtime) *goja.Object {
	e := vm.NewObject()
	e.Set("base64Encode", func(data string) string { return base64.StdEncoding.EncodeToString([]byte(data)) })
	e.Set("base64Decode", func(s string) map[string]any {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": string(b)}
	})
	e.Set("urlEncode", func(s string) string { return url.QueryEscape(s) })
	e.Set("urlDecode", func(s string) map[string]any {
		out, err := url.QueryUnescape(s)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": out}
	})
	e.Set("lunarEncoder", func(key, content string) map[string]any {
		out, err := encodingLunarEncoder(key, content)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": out}
	})
	e.Set("lunarDecoder", func(key, cipher string) map[string]any {
		out, err := encodingLunarDecoder(key, cipher)
		if err != nil {
			return map[string]any{"success": false, "error": err.Error()}
		}
		return map[string]any{"success": true, "text": out}
	})
	return e
}

// encodingLunarEncoder 加密：把明文经 lunar_decoder 以密钥字符串加密后返回密文。
func encodingLunarEncoder(key, content string) (string, error) {
	outs, err := lunardecoder.EncodeFilesWithKeyString([]lunardecoder.FileData{{Name: "m", Data: []byte(content)}}, key)
	if err != nil {
		return "", err
	}
	if len(outs) == 0 {
		return "", fmt.Errorf("加密结果为空")
	}
	return string(outs[0].Data), nil
}

// encodingLunarDecoder 解密：把密文经 lunar_decoder 以密钥字符串解密后返回原文。
func encodingLunarDecoder(key, cipher string) (string, error) {
	outs, err := lunardecoder.DecodeFilesWithKeyString([]lunardecoder.FileData{{Name: "m", Data: []byte(cipher)}}, key)
	if err != nil {
		return "", err
	}
	if len(outs) == 0 {
		return "", fmt.Errorf("解码结果为空")
	}
	return string(outs[0].Data), nil
}
