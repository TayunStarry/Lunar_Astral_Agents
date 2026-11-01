package runtime

// ==== encoding 编解码 ====
// 提供 base64 / url 编解码。
// 入参与返回均为字符串（UTF-8 文本）；二进制数据如需传递可自行用 base64 包裹。

import (
	"encoding/base64"
	"net/url"

	"github.com/dop251/goja"
)

// bindEncoding encoding.base64Encode/base64Decode/urlEncode/urlDecode。
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
	return e
}
