package StarLTP

// ==== engine.* 扩展能力实现：JWT / 图像 / LLM / 发送 / WebSocket ====
// 由宿主可选注入真实通道（发送、WebSocket 传输）或读取 lunar_config.json（LLM）。

import (
	"bytes"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/GeneralConfig"

	"github.com/dop251/goja"
)

// ==== JWT 签名 engine.crypto.signJWT（allow-certificate） ====

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

// parseEd25519Key 解析 PEM（PKCS8 Ed25519）私钥；兼容无 PEM 包裹的原生私钥字节。
func parseEd25519Key(keyStr string) (ed25519.PrivateKey, error) {
	raw := []byte(keyStr)
	if block, _ := pem.Decode(raw); block != nil {
		raw = block.Bytes
	}
	key, err := x509.ParsePKCS8PrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("Ed25519 私钥解析失败: %v", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是 Ed25519 类型")
	}
	return priv, nil
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

// engineImageDownload 下载 url 到插件数据目录 fileName，校验 magic 后返回 base64（allow-network）。
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

func engineSend(kind, target string, payload any) (any, error) {
	sendMu.RLock()
	inv := sendInvoker
	sendMu.RUnlock()
	if inv == nil {
		return rwResult{Success: false, Error: "未注入发送通道"}, nil
	}
	if err := inv(kind, target, payload); err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	return rwResult{Success: true}, nil
}

// ==== LLM engine.llm.chat（allow-agent，读取 lunar_config.json 的 agent 字段） ====

// readAgentConfig 从本地 lunar_config.json 读取 agent 的 multimodal 配置（不硬编码模型名）。
func readAgentConfig() (baseURL, apiKey, model string) {
	execPath, err := os.Executable()
	if err != nil {
		return "", "", ""
	}
	raw, rerr := os.ReadFile(filepath.Join(filepath.Dir(execPath), *GeneralConfig.LocalDir, "lunar_config.json"))
	if rerr != nil {
		return "", "", ""
	}
	var cfg struct {
		Agent struct {
			Model string `json:"multimodal_model"`
			URL   string `json:"multimodal_url"`
			Key   string `json:"multimodal_key"`
		} `json:"agent"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return "", "", ""
	}
	return cfg.Agent.URL, cfg.Agent.Key, cfg.Agent.Model
}

// engineLLMChat 调用 OpenAI 兼容 v1/chat/completions，返回首个 choice 的文本内容。
func engineLLMChat(messages []any, opts map[string]any) (any, error) {
	baseURL, apiKey, model := readAgentConfig()
	if model == "" {
		model = "system-multimodal"
	}
	if baseURL == "" {
		return rwResult{Success: false, Error: "未配置 agent.multimodal_url（lunar_config.json）"}, nil
	}
	payload := map[string]any{"model": model, "messages": messages, "stream": false}
	if t, ok := opts["temperature"].(float64); ok {
		payload["temperature"] = t
	}
	if topt, ok := opts["max_tokens"].(int64); ok {
		payload["max_tokens"] = topt
	}
	body, _ := json.Marshal(payload)
	// multimodal_url 自身（已含 /v1，如 http://host:port/v1）为服务根，据此拼出正确的 chat/completions 路径
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, rerr := http.NewRequest("POST", base+"/chat/completions", bytes.NewReader(body))
	if rerr != nil {
		return rwResult{Success: false, Error: rerr.Error()}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, rerr := client.Do(req)
	if rerr != nil {
		return rwResult{Success: false, Error: "LLM 请求失败: " + rerr.Error()}, nil
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return rwResult{Success: false, Error: fmt.Sprintf("LLM 状态 %d: %s", resp.StatusCode, previewStr(rb, 300))}, nil
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(rb, &out) != nil {
		return rwResult{Success: false, Error: "LLM 响应解析失败"}, nil
	}
	if len(out.Choices) == 0 {
		return rwResult{Success: false, Error: "LLM 无输出"}, nil
	}
	return rwResult{Success: true, Text: out.Choices[0].Message.Content}, nil
}

func previewStr(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ==== WebSocket engine.ws（allow-socket，传输由宿主注入的 WsBridge 提供） ====

// engineWsServe 委托宿主挂载一个 WebSocket 端点；onMessage 内的 goja 回调在插件 loop 内执行。
func engineWsServe(p *plugin, path string, handler jsFunc) (any, error) {
	wsMu.RLock()
	bridge := wsServicer
	wsMu.RUnlock()
	if bridge == nil {
		return rwResult{Success: false, Error: "未注入 WebSocket 传输"}, nil
	}
	addr, err := bridge.Serve(p.ID, path, func(msg string) string {
		var res string
		p.loop.RunOnLoop(func(vm *goja.Runtime) {
			f, ok := goja.AssertFunction(handler)
			if !ok {
				res = "handler not a function"
				return
			}
			v, cerr := f(goja.Undefined(), vm.ToValue(msg))
			if cerr != nil {
				res = "handler error: " + cerr.Error()
				return
			}
			res = v.String()
		})
		return res
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
