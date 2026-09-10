package StarLTP

// ==== engine.encoding 编解码（常驻基础能力，不参与 allow-* 开关） ====
// 提供 base64 / hex / url / utf8 四种编解码。入参与返回均为字符串（UTF-8 文本），
// 与 LTP9 全字符串模型一致；二进制数据如需传递可自行用 base64 包裹。

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"

	"github.com/dop251/goja"
)

// bindEncoding engine.encoding.*：纯编解码工具，常驻注入。
func bindEncoding(vm *goja.Runtime) *goja.Object {
	e := vm.NewObject()
	e.Set("base64Encode", func(data string) string { return base64.StdEncoding.EncodeToString([]byte(data)) })
	e.Set("base64Decode", func(s string) (string, error) {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
	e.Set("hexEncode", func(data string) string { return hex.EncodeToString([]byte(data)) })
	e.Set("hexDecode", func(s string) (string, error) {
		b, err := hex.DecodeString(s)
		if err != nil {
			return "", err
		}
		return string(b), nil
	})
	e.Set("urlEncode", func(s string) string { return url.QueryEscape(s) })
	e.Set("urlDecode", func(s string) (string, error) {
		out, err := url.QueryUnescape(s)
		if err != nil {
			return "", err
		}
		return out, nil
	})
	// utf8 仅做原文直通（LTP9 内部即 UTF-8 字符串），保留以对齐 LTP3 的 encoding 形态。
	e.Set("utf8Encode", func(s string) string { return s })
	e.Set("utf8Decode", func(s string) string { return s })
	return e
}