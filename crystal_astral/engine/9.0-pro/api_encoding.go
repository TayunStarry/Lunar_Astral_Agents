package ltp9

// ==== engine.encoding 编解码（常驻基础能力，不参与 allow-* 开关） ====

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"unicode/utf8"

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
	e.Set("utf8Encode", func(s string) []int {
		out := make([]int, 0, len(s))
		for _, r := range s {
			buf := make([]byte, utf8.RuneLen(r))
			n := utf8.EncodeRune(buf, r)
			for i := 0; i < n; i++ {
				out = append(out, int(buf[i]))
			}
		}
		return out
	})
	e.Set("utf8Decode", func(v goja.Value) string { return string(valueToBytes(v)) })
	return e
}