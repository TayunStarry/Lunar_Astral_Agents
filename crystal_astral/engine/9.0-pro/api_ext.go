package ltp9

// ==== engine.* 扩展能力实现：JWT / 图像 / 发送 / 平台 / WebSocket 服务端 ====

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
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// ==== JWT 签名 engine.crypto.signJWT（allow-certificate） ====

func b64UrlRaw(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// engineSignJWT 生成 JWT 令牌：支持 HS256（默认）、EdDSA（Ed25519）与 none。
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

// parseEd25519Key 解析 Ed25519 私钥。兼容 PKCS8 PEM、base64 DER 与裸 32 字节种子。
func parseEd25519Key(keyStr string) (ed25519.PrivateKey, error) {
	input := strings.TrimSpace(keyStr)
	raw := []byte(input)

	if block, _ := pem.Decode(raw); block != nil {
		return parsePKCS8Ed25519(block.Bytes)
	}

	if b64, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input)); err == nil {
		if len(b64) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(b64), nil
		}
		if k, kerr := parsePKCS8Ed25519(b64); kerr == nil {
			return k, nil
		}
	}

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

// ==== 摘要 / HMAC / Ed25519 / JWT 原语 engine.crypto.*（allow-certificate） ====

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

// edgeJWT 用 Ed25519 生成 EdDSA JWT。
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

// ==== 图像处理 engine.image（allow-file：读/校验；allow-network：下载） ====

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

// engineImageLoadValid 读取插件数据目录内的图片，校验 magic 后返回 base64。
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

// engineImageDownload 下载 url 到插件数据目录 fileName，校验 magic 后返回 base64。
func engineImageDownload(p *plugin, url, fileName string) (any, error) {
	real, err := scopedPath(p, fileName)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, derr := client.Get(url)
	if derr != nil {
		return rwResult{Success: false, Error: "下载失败: " + derr.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return rwResult{Success: false, Error: "下载状态: " + resp.Status}, nil
	}
	b, rerr := io.ReadAll(resp.Body)
	if rerr != nil || !isImageContent(b) {
		return rwResult{Success: false, Error: "下载内容非有效图片（链接可能过期）"}, nil
	}
	if werr := os.WriteFile(real, b, 0644); werr != nil {
		return rwResult{Success: false, Error: werr.Error()}, nil
	}
	return rwResult{Success: true, Text: base64.StdEncoding.EncodeToString(b)}, nil
}

// ==== 发送 engine.send（allow-send，通道由宿主注入） ====

// engineSend 把发送请求转交给宿主注入的 sendInvoker。
func engineSend(pluginID, kind, target string, payload any) (any, error) {
	sendMu.RLock()
	inv := sendInvoker
	sendMu.RUnlock()
	if inv == nil {
		return rwResult{Success: false, Error: "未注入发送通道"}, nil
	}
	if err := inv(pluginID, kind, target, payload); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return rwResult{Success: true}, nil
}

// ==== 平台上下文 engine.platform（allow-send，由宿主注入 platformResolver） ====

// enginePlatform 委托宿主解析平台上下文。
func enginePlatform(method string, args map[string]any) (any, error) {
	platformMu.RLock()
	resolver := platformResolver
	platformMu.RUnlock()
	if resolver == nil {
		return rwResult{Success: false, Error: "未接入平台（宿主未注入 platformResolver）"}, nil
	}
	v, err := resolver(method, args)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return v, nil
}

// ==== 工具调用 CallTool（引擎侧，供宿主经 tool 或 AtoA 接头调用） ====

// engineToolCall 调用目标插件注册的工具。context 透传给工具 handler 第二参数。
func engineToolCall(id, name string, args map[string]any, context map[string]any) (any, error) {
	if Engine == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	p := Engine.pluginFor(id)
	if p == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	return p.callTool(name, args, context)
}

func previewStr(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ==== WebSocket engine.ws（allow-socket，传输由宿主注入的 WsBridge 提供） ====

// wsHandlerTimeout 插件 WS handler 的单次执行超时。
const wsHandlerTimeout = 10 * time.Second

// engineWsServe 委托宿主挂载一个 WebSocket 端点。
func engineWsServe(p *plugin, path string, handler jsFunc) (any, error) {
	wsMu.RLock()
	bridge := wsServicer
	wsMu.RUnlock()
	if bridge == nil {
		return rwResult{Success: false, Error: "未注入 WebSocket 传输"}, nil
	}
	addr, err := bridge.Serve(p.ID, path, func(msg string) string {
		resCh := make(chan string, 1)
		loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
			f, ok := goja.AssertFunction(handler)
			if !ok {
				resCh <- "handler not a function"
				return
			}
			v, cerr := f(goja.Undefined(), vm.ToValue(msg))
			if cerr != nil {
				resCh <- "handler error: " + cerr.Error()
				return
			}
			resCh <- v.String()
		})
		if !loopOK {
			return "plugin loop terminated"
		}
		select {
		case res := <-resCh:
			return res
		case <-time.After(wsHandlerTimeout):
			return "handler timeout"
		}
	})
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return map[string]any{"success": true, "path": addr}, nil
}

// engineWsPublish 向本插件挂载的 WebSocket 路径广播数据。
func engineWsPublish(p *plugin, data string) (any, error) {
	wsMu.RLock()
	bridge := wsServicer
	wsMu.RUnlock()
	if bridge == nil {
		return rwResult{Success: false, Error: "未注入 WebSocket 传输"}, nil
	}
	if err := bridge.Publish(p.ID, "", data); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return rwResult{Success: true}, nil
}