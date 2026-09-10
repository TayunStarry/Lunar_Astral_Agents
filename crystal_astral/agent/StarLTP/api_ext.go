package StarLTP

// ==== engine.* 扩展能力实现：JWT / 图像 / LLM / 发送 / WebSocket ====
// 由宿主可选注入真实通道（发送、WebSocket 传输）或读取 lunar_config.json（LLM）。

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

// parseEd25519Key 解析 Ed25519 私钥。兼容以下表示：
//  1. PKCS8 PEM（"-----BEGIN PRIVATE KEY-----"）；
//  2. base64 编码的 DER（PKCS8 或裸 32 字节种子）；
//  3. 裸 32 字节种子。
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

// ==== 摘要 / HMAC / Ed25519 / JWT 原语 engine.crypto.*（allow-certificate） ====

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

// engineLLMChat 调用 OpenAI 兼容 v1/chat/completions，返回首个 choice 的文本内容。
// 配置统一读取 GeneralConfig 的 agent.multimodal_*（来自 lunar_config.json，与 embed/memory 同源）。
func engineLLMChat(messages []any, opts map[string]any) (any, error) {
	model := stringOr(*GeneralConfig.AgentMultimodalModel, "system-multimodal")
	baseURL := strings.TrimSpace(*GeneralConfig.AgentMultimodalURL)
	apiKey := *GeneralConfig.AgentMultimodalKey
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
	// 系统提示词：以 opts.system 传入时前置为一条 system 消息（不改动原 messages）
	if sys, ok := opts["system"].(string); ok && sys != "" {
		prepended := make([]any, 0, len(messages)+1)
		prepended = append(prepended, map[string]any{"role": "system", "content": sys})
		prepended = append(prepended, messages...)
		payload["messages"] = prepended
	}
	// tools：支持给 LLM 提供函数/工具定义（AtoA 智能体需要给模型接头）
	if tools, ok := opts["tools"].([]any); ok && len(tools) > 0 {
		payload["tools"] = tools
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
				Content   string `json:"content"`
				ToolCalls []any  `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(rb, &out) != nil {
		return rwResult{Success: false, Error: "LLM 响应解析失败"}, nil
	}
	if len(out.Choices) == 0 {
		return rwResult{Success: false, Error: "LLM 无输出"}, nil
	}
	res := rwResult{Success: true, Text: out.Choices[0].Message.Content}
	// 模型走函数调用（tool_calls）时 content 为空，把 tool_calls 原样回传供 AtoA 智能体继续执行
	if len(out.Choices[0].Message.ToolCalls) > 0 {
		res.ToolCalls = out.Choices[0].Message.ToolCalls
	}
	return res, nil
}

// readEmbedConfig 从项目配置读取智能体文本嵌入模型配置（lunar_config.json 的 agent 字段，不硬编码）。
func readEmbedConfig() (model, baseURL, apiKey string) {
	return stringOr(*GeneralConfig.AgentEmbeddingModel, "system-embedding"),
		stringOr(*GeneralConfig.AgentEmbeddingURL, "http://127.0.0.1:36789/v1"),
		stringOr(*GeneralConfig.AgentEmbeddingKey, "")
}

// embedComplete 调用 OpenAI 兼容 v1/embeddings 批量嵌入文本，返回与输入等长的向量列表。
func embedComplete(texts []string, model, baseURL, apiKey string) ([][]float64, error) {
	bodyMap := map[string]any{"model": model, "input": texts}
	body, _ := json.Marshal(bodyMap)
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(base, "/v1") {
		base += "/v1"
	}
	req, rerr := http.NewRequest("POST", base+"/embeddings", bytes.NewReader(body))
	if rerr != nil {
		return nil, rerr
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, rerr := client.Do(req)
	if rerr != nil {
		return nil, fmt.Errorf("嵌入请求失败: %v", rerr)
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("嵌入状态 %d: %s", resp.StatusCode, previewStr(rb, 300))
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
		Error *struct{ Message string `json:"message"` } `json:"error"`
	}
	if json.Unmarshal(rb, &out) != nil {
		return nil, fmt.Errorf("嵌入响应解析失败")
	}
	if out.Error != nil {
		return nil, fmt.Errorf("嵌入错误: %s", out.Error.Message)
	}
	vecs := make([][]float64, 0, len(out.Data))
	for _, d := range out.Data {
		vecs = append(vecs, d.Embedding)
	}
	if len(vecs) != len(texts) {
		return nil, fmt.Errorf("嵌入响应数量不匹配: 期望 %d, 实际 %d", len(texts), len(vecs))
	}
	return vecs, nil
}

// engineLLMEmbed 计算文本的嵌入向量（engine.llm.embed，allow-agent）。
// input 可为单条字符串或字符串数组；返回 { success, embedding } 或 { success, embeddings }。
func engineLLMEmbed(input any, opts map[string]any) (any, error) {
	texts := []string{}
	// 位置入参优先：字符串 或 字符串数组
	switch v := input.(type) {
	case string:
		texts = append(texts, v)
	case []any:
		for _, s := range v {
			if st, ok := s.(string); ok && st != "" {
				texts = append(texts, st)
			}
		}
	}
	// 位置入参为空时，用 opts.text / opts.texts 兜底（避免与入参叠加成双文本）
	if len(texts) == 0 && opts != nil {
		if t, ok := opts["text"].(string); ok && t != "" {
			texts = []string{t}
		} else if arr, ok := opts["texts"].([]any); ok {
			for _, s := range arr {
				if st, ok := s.(string); ok && st != "" {
					texts = append(texts, st)
				}
			}
		}
	}
	if len(texts) == 0 {
		return rwResult{Success: false, Error: "缺少待嵌入文本（需传入 string / []string，或 opts.text / opts.texts）"}, nil
	}
	model, url, key := readEmbedConfig()
	if url == "" {
		return rwResult{Success: false, Error: "未配置 agent.embedding_url（lunar_config.json）"}, nil
	}
	vecs, err := embedComplete(texts, model, url, key)
	if err != nil {
		return rwResult{Success: false, Error: err.Error()}, nil
	}
	if len(vecs) == 1 {
		return map[string]any{"success": true, "embedding": vecs[0]}, nil
	}
	return map[string]any{"success": true, "embeddings": vecs}, nil
}

// stringOr 返回第一个非空值，全空返回默认。
func stringOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// ==== 平台上下文 engine.platform（allow-send，由宿主注入 platformResolver） ====

// enginePlatform 委托宿主解析平台上下文（getName / getGroupId / lookupUser）。未注入时返回错误提示。
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

// ==== 工具调用 CallTool（引擎侧，供宿主经 ltp9/tool 或 AtoA 接头调用） ====

// engineToolCall 调用目标插件注册的工具（等价 engine.tool）。未注册返回错误。
func engineToolCall(id, name string, args map[string]any) (any, error) {
	if Engine == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	p := Engine.pluginFor(id)
	if p == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", id)
	}
	return p.callTool(name, args)
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
