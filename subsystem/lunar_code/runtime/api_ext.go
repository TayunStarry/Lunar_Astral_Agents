package runtime

// ==== 扩展能力实现：JWT / 摘要 / Ed25519 / 图像 / LLM ====
// LLM 配置读取 config/general.json 的 agent 字段（engine/config 同源），不硬编码。

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

// ==== JWT 签名（hash.signJWT） ====

func b64UrlRaw(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// engineSignJWT 生成 JWT 令牌：支持 HS256（默认）、EdDSA（Ed25519）与 none。claims 为令牌负载对象，
// kid 可选，非空时写入 JWT 头（用于 QWeather 等多密钥场景按 Key ID 选择签名密钥）。
func engineSignJWT(claims map[string]any, secret, algorithm, kid string) (string, error) {
	if algorithm == "" {
		algorithm = "HS256"
	}
	if algorithm != "HS256" && algorithm != "EdDSA" && algorithm != "none" {
		return "", fmt.Errorf("不支持的 JWT 算法: %s", algorithm)
	}
	header := map[string]any{"alg": algorithm, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	hdr, _ := json.Marshal(header)
	load, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := b64UrlRaw(hdr) + "." + b64UrlRaw(load)
	if algorithm == "none" {
		return signingInput + ".", nil
	}
	if algorithm == "EdDSA" {
		key, derr := parseEd25519Key(secret)
		if derr != nil {
			return "", derr
		}
		sig := ed25519.Sign(key, []byte(signingInput))
		return signingInput + "." + b64UrlRaw(sig), nil
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return signingInput + "." + b64UrlRaw(mac.Sum(nil)), nil
}

// parseEd25519Key 解析 Ed25519 私钥。兼容以下表示：
//  1. PKCS8 PEM（"-----BEGIN PRIVATE KEY-----"）；
//  2. base64 编码的 DER（PKCS8 或裸 32 字节种子）；
//  3. 裸 32 字节种子。
//
// 均失败时返回带密钥长度的可读错误，便于定位配置问题。
func parseEd25519Key(keyStr string) (ed25519.PrivateKey, error) {
	input := strings.TrimSpace(keyStr)
	raw := []byte(input)

	// 1) PEM（PKCS8）
	if block, _ := pem.Decode(raw); block != nil {
		return parsePKCS8Ed25519(block.Bytes)
	}

	// 2) base64 编码的 DER：先试裸种子（正好 32 字节），再试 PKCS8
	if b64, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input)); err == nil {
		if len(b64) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(b64), nil
		}
		if k, kerr := parsePKCS8Ed25519(b64); kerr == nil {
			return k, nil
		}
	}

	// 3) 裸 32 字节种子
	if len(raw) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(raw), nil
	}

	return nil, fmt.Errorf("Ed25519 私钥解析失败: 无法识别的格式（需 PKCS8 PEM、base64 DER 或 32 字节种子），实际密钥长度为 %d 字节", len(raw))
}

// parsePKCS8Ed25519 从 DER 字节解析 PKCS8 Ed25519 私钥。
func parsePKCS8Ed25519(der []byte) (ed25519.PrivateKey, error) {
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("PKCS8 私钥解析失败: %v", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是 Ed25519 类型")
	}
	return priv, nil
}

// ==== 摘要 / HMAC / Ed25519 / JWT 原语（hash.*） ====

// cryptoMD5 / cryptoSHA1 / cryptoSHA256 摘要为小写十六进制字符串。
func cryptoMD5(data string) string {
	h := md5.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

func cryptoSHA1(data string) string {
	h := sha1.Sum([]byte(data))
	return hex.EncodeToString(h[:])
}

func cryptoSHA256(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// cryptoHMAC1 / cryptoHMAC256 计算 HMAC 并返回小写十六进制字符串。
func cryptoHMAC1(key, data string) string {
	m := hmac.New(sha1.New, []byte(key))
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}

func cryptoHMAC256(key, data string) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(data))
	return hex.EncodeToString(m.Sum(nil))
}

// edgeSign 用 Ed25519 私钥对数据签名，返回 base64url。
func edgeSign(privStr, data string) (string, error) {
	key, err := parseEd25519Key(privStr)
	if err != nil {
		return "", err
	}
	return b64UrlRaw(ed25519.Sign(key, []byte(data))), nil
}

// edgeJWT 用 Ed25519 生成 EdDSA JWT（kid 可选写入头）。
func edgeJWT(privStr, kid string, claims map[string]any) (string, error) {
	key, err := parseEd25519Key(privStr)
	if err != nil {
		return "", err
	}
	header := map[string]any{"alg": "EdDSA"}
	if kid != "" {
		header["kid"] = kid
	}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	m := b64UrlRaw(hb) + "." + b64UrlRaw(cb)
	return m + "." + b64UrlRaw(ed25519.Sign(key, []byte(m))), nil
}

// ==== 图像处理 image.loadValid/download ====

func isImageContent(b []byte) bool {
	if len(b) >= 8 && bytes.Equal(b[:4], []byte("\x89PNG")) {
		return true
	}
	if len(b) >= 3 && bytes.Equal(b[:3], []byte{0xFF, 0xD8, 0xFF}) {
		return true
	}
	if len(b) >= 4 && bytes.HasPrefix(b, []byte("GIF8")) {
		return true
	}
	if len(b) >= 12 && bytes.HasPrefix(b, []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return true
	}
	if len(b) >= 2 && bytes.HasPrefix(b, []byte("BM")) {
		return true
	}
	return false
}

// engineImageLoadValid 读取插件数据目录内的图片，校验 magic 后返回 base64 明文（未编码字符串）。
func engineImageLoadValid(p *plugin, path string) (any, error) {
	real, err := scopedPath(p, path)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	b, rerr := os.ReadFile(real)
	if rerr != nil || !isImageContent(b) {
		return rwResult{Success: false, Error: "非有效图片或读取失败: " + path}, nil
	}
	return rwResult{Success: true, Text: base64.StdEncoding.EncodeToString(b)}, nil
}

// ==== LLM agent.chat（智能体实现，装配层注入） ====

// engineLLMChat 调用通用推理智能体（实现于 agents/RuntimeChat），返回正文与工具调用。
func engineLLMChat(messages []any, opts map[string]any) (any, error) {
	if AgentLLMFunc == nil {
		return rwResult{Success: false, Error: "LLM 智能体未装配"}, nil
	}
	text, toolCalls, err := AgentLLMFunc(messages, opts)
	if err != nil {
		return rwResult{Success: false, Error: "LLM 请求失败: " + err.Error()}, nil
	}
	res := rwResult{Success: true, Text: text}
	// 模型走函数调用（tool_calls）时 content 为空，把 tool_calls 原样回传供 AtoA 智能体继续执行
	if len(toolCalls) > 0 {
		res.ToolCalls = toolCalls
	}
	return res, nil
}
