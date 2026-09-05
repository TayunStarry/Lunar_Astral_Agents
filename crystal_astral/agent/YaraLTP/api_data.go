package YaraLTP

// ==== 纯计算 API：encoding / time / crypto ====

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// bindEncoding 注入 yara.encoding（返回 number[] 以对齐 d.ts）。
func bindEncoding(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "base64Encode", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		return vm.ToValue(base64.StdEncoding.EncodeToString(b))
	})
	objSetFn(o, "base64Decode", func(call goja.FunctionCall) goja.Value {
		raw := strings.TrimSpace(argString(call, 0))
		raw = strings.ReplaceAll(raw, "\n", "")
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return vm.ToValue([]any{})
		}
		return vm.ToValue(intSlice(b))
	})
	objSetFn(o, "hexEncode", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		return vm.ToValue(hex.EncodeToString(b))
	})
	objSetFn(o, "hexDecode", func(call goja.FunctionCall) goja.Value {
		b, err := hex.DecodeString(strings.TrimSpace(argString(call, 0)))
		if err != nil {
			return vm.ToValue([]any{})
		}
		return vm.ToValue(intSlice(b))
	})
	objSetFn(o, "urlEncode", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(urlEncode(argString(call, 0)))
	})
	objSetFn(o, "urlDecode", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(urlDecode(argString(call, 0)))
	})
	objSetFn(o, "utf8Encode", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(intSlice([]byte(argString(call, 0))))
	})
	objSetFn(o, "utf8Decode", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		return vm.ToValue(string(b))
	})
	parent.Set("encoding", o)
}

// bindTime 注入 yara.time。
func bindTime(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "now", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().Unix())
	})
	objSetFn(o, "nowMs", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(time.Now().UnixMilli())
	})
	objSetFn(o, "format", func(call goja.FunctionCall) goja.Value {
		ts := argInt(call, 0)
		layout := argString(call, 1)
		if layout == "" {
			layout = "2006-01-02 15:04:05"
		}
		return vm.ToValue(time.Unix(ts, 0).Format(layout))
	})
	objSetFn(o, "formatDuration", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(formatDuration(int64(argInt(call, 0))))
	})
	objSetFn(o, "parse", func(call goja.FunctionCall) goja.Value {
		str := argString(call, 0)
		layout := argString(call, 1)
		if layout == "" {
			layout = "2006-01-02 15:04:05"
		}
		t, err := time.ParseInLocation(layout, str, time.Local)
		if err != nil {
			return vm.ToValue(nil)
		}
		return vm.ToValue(t.Unix())
	})
	objSetFn(o, "sleep", func(call goja.FunctionCall) goja.Value {
		ms := argInt(call, 0)
		if ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		return goja.Undefined()
	})
	parent.Set("time", o)
}

// bindCrypto 注入 yara.crypto。
func bindCrypto(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "md5", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		h := md5.Sum(b)
		return vm.ToValue(hex.EncodeToString(h[:]))
	})
	objSetFn(o, "sha1", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		h := sha1.Sum(b)
		return vm.ToValue(hex.EncodeToString(h[:]))
	})
	objSetFn(o, "sha256", func(call goja.FunctionCall) goja.Value {
		b, _ := toBytes(argExport(call, 0))
		h := sha256.Sum256(b)
		return vm.ToValue(hex.EncodeToString(h[:]))
	})
	objSetFn(o, "hmacSha1", func(call goja.FunctionCall) goja.Value {
		key, _ := toBytes(argExport(call, 0))
		data, _ := toBytes(argExport(call, 1))
		m := hmac.New(sha1.New, key)
		m.Write(data)
		return vm.ToValue(hex.EncodeToString(m.Sum(nil)))
	})
	objSetFn(o, "hmacSha256", func(call goja.FunctionCall) goja.Value {
		key, _ := toBytes(argExport(call, 0))
		data, _ := toBytes(argExport(call, 1))
		m := hmac.New(sha256.New, key)
		m.Write(data)
		return vm.ToValue(hex.EncodeToString(m.Sum(nil)))
	})
	objSetFn(o, "ed25519Sign", func(call goja.FunctionCall) goja.Value {
		priv, err := ed25519PrivFromArg(argExport(call, 0))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		data, _ := toBytes(argExport(call, 1))
		sig := ed25519.Sign(priv, data)
		return vm.ToValue(base64.RawURLEncoding.EncodeToString(sig))
	})
	objSetFn(o, "generateJWT", func(call goja.FunctionCall) goja.Value {
		claims := argMap(call, 0)
		priv, err := ed25519PrivFromArg(argExport(call, 1))
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		keyID := argString(call, 2)
		tok, err := jwtSign(priv, keyID, claims)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(tok)
	})
	parent.Set("crypto", o)
}

// ed25519PrivFromArg 把 PEM(PKCS8，含单行字面 "\n" 形态) 或原始 32/64 字节私钥
// 转为 ed25519.PrivateKey。PKCS8 用 crypto/x509 标准解析，兼容短 DER（如 MC4C... 形式）。
func ed25519PrivFromArg(v any) (ed25519.PrivateKey, error) {
	s := toString(v)
	s = strings.ReplaceAll(strings.TrimSpace(s), "\\n", "\n")
	b, _ := toBytes(v)
	if strings.Contains(s, "BEGIN") {
		der, err := parsePEM(s)
		if err != nil {
			return nil, err
		}
		return keyFromDER(der)
	}
	return keyFromDER(b)
}

// keyFromDER 从 DER 字节解析 ed25519 私钥：优先 PKCS8，其次按 32/64 字节原始 seed。
func keyFromDER(der []byte) (ed25519.PrivateKey, error) {
	if len(der) > 0 {
		if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
			if pk, ok := k.(ed25519.PrivateKey); ok {
				return pk, nil
			}
		}
	}
	if len(der) == 64 {
		return ed25519.NewKeyFromSeed(der[:32]), nil
	}
	if len(der) == 32 {
		return ed25519.NewKeyFromSeed(der), nil
	}
	return nil, fmt.Errorf("无效的 Ed25519 私钥格式")
}

// parsePEM 提取 PKCS8 PRIVATE KEY 块内的 DER。
// 兼容真实多行 PEM 与单行内用字面 "\n" 转义的多行 PEM（插件 config.yaml 常见形态）。
func parsePEM(pemStr string) ([]byte, error) {
	pemStr = strings.ReplaceAll(pemStr, "\\n", "\n")
	var b64 strings.Builder
	for _, ln := range strings.Split(pemStr, "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "-----") {
			continue
		}
		b64.WriteString(ln)
	}
	raw, err := base64.StdEncoding.DecodeString(b64.String())
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// jwtSign 用 Ed25519 生成 JWT（EdDSA）。
func jwtSign(priv ed25519.PrivateKey, keyID string, claims map[string]any) (string, error) {
	header := map[string]any{"alg": "EdDSA"}
	if keyID != "" {
		header["kid"] = keyID
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	h := base64.RawURLEncoding.EncodeToString(hb)
	c := base64.RawURLEncoding.EncodeToString(cb)
	m := h + "." + c
	sig := ed25519.Sign(priv, []byte(m))
	s := base64.RawURLEncoding.EncodeToString(sig)
	return m + "." + s, nil
}

// formatDuration 秒 → 人类可读中文时长。
func formatDuration(sec int64) string {
	if sec < 60 {
		return fmt.Sprintf("%d秒", sec)
	}
	days := sec / 86400
	hours := (sec % 86400) / 3600
	mins := (sec % 3600) / 60
	secs := sec % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d天", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d小时", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%d分钟", mins))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%d秒", secs))
	}
	return strings.Join(parts, "")
}

// urlEncode 百分号编码（非 ASCII 与保留字符转义）。
func urlEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isURLSafe(c) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// isURLSafe 判断 URL 编解码时保持原样的字节。
func isURLSafe(c byte) bool {
	if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
		return true
	}
	return strings.IndexByte("-_.~", c) >= 0
}

// urlDecode 百分号解码。
func urlDecode(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '%' && i+2 < len(s)+1 && i+2 <= len(s)-1 {
			hi, hok := fromHex(s[i+1])
			lo, lok := fromHex(s[i+2])
			if hok && lok {
				b = append(b, hi<<4|lo)
				i += 2
				continue
			}
			b = append(b, c)
		} else if c == '+' {
			b = append(b, ' ')
		} else {
			b = append(b, c)
		}
	}
	return string(b)
}

func fromHex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
