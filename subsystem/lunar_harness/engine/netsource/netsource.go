package netsource

// netsource.go 本机资源定位与统一回源客户端
//
// 前端上传后返回 `${origin}/file/read/<relative>`：桌面端 origin 是回环地址，
// 手机端是局域网 IP（经自签 HTTPS 代理访问）。两者指向的都是本机服务，
// 文件也已落在 LocalDir 下，必须直接读盘，不能回源导致自签证书校验失败。
//
// 远程地址的回源统一走这里构建的客户端：默认保留证书校验，
// 只有本机 / 局域网地址自动豁免；显式开启 config.InsecureTLS 时才放开远程校验。

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/proxy"
)

// IsLocalServiceHost 判断主机是否指向本机服务（回环 / localhost / 本机网卡地址）
func IsLocalServiceHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(strings.Trim(host, "[]")))
	if host == "" {
		return false
	}
	switch host {
	case "127.0.0.1", "localhost", "::1", "0.0.0.0":
		return true
	}
	for _, ip := range proxy.LocalIPs() {
		if host == ip {
			return true
		}
	}
	return false
}

// FileReadRel 从本机服务地址或裸读取路由中提取相对 LocalDir 的路径（不校验文件是否存在）
func FileReadRel(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil || !IsLocalServiceHost(parsed.Hostname()) {
			return "", false
		}
		return trimReadRoute(parsed.Path)
	}
	return trimReadRoute(strings.ReplaceAll(raw, "\\", "/"))
}

// trimReadRoute 去掉本机读取路由前缀并解码路径
func trimReadRoute(p string) (string, bool) {
	prefixes := []string{
		LocalFileReadPrefix,
		strings.TrimPrefix(LocalFileReadPrefix, "/"),
		"/file/miniature/",
		"file/miniature/",
	}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rel := strings.TrimPrefix(p, prefix)
		if rel == "" {
			return "", false
		}
		decoded, err := url.PathUnescape(rel)
		if err != nil {
			return "", false
		}
		return decoded, true
	}
	return "", false
}

// LocalPath 把本机服务地址或裸读取路由解析为 LocalDir 下已存在的绝对路径
func LocalPath(raw string) (string, bool) {
	rel, ok := FileReadRel(raw)
	if !ok {
		return "", false
	}
	abs, err := UnderLocalDir(rel)
	if err != nil {
		return "", false
	}
	if _, err := os.Stat(abs); err != nil {
		return "", false
	}
	return abs, true
}

// UnderLocalDir 解析相对 LocalDir 的路径并做穿越校验
func UnderLocalDir(rel string) (string, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.TrimPrefix(rel, "/")
	localDir := filepath.Clean(config.LocalDir)
	if filepath.IsAbs(rel) {
		abs := filepath.Clean(rel)
		if !strings.HasPrefix(abs, localDir+string(os.PathSeparator)) && abs != localDir {
			return "", fmt.Errorf("路径超出本地目录范围: %s", rel)
		}
		return abs, nil
	}
	abs := filepath.Clean(filepath.Join(localDir, filepath.FromSlash(rel)))
	if !strings.HasPrefix(abs, localDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径穿越: %s", rel)
	}
	return abs, nil
}

// ShouldSkipVerify 判断某次回源是否跳过 TLS 证书校验
//   - 本机 / 局域网地址：自签 HTTPS 代理场景，自动豁免（作用范围仅限本机地址）
//   - 远程地址：默认严格校验，仅显式开启 server.insecure_tls（config.InsecureTLS）时豁免
func ShouldSkipVerify(rawURL string) bool {
	host := ""
	if parsed, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		host = parsed.Hostname()
	}
	if IsLocalServiceHost(host) {
		return true
	}
	return config.InsecureTLS
}

// NewHTTPClient 构建统一配置的 HTTP 客户端（超时 / 重定向上限 / 可选跳过证书校验）
// 不跟随代理环境变量；需要代理的调用方用 NewHTTPClientOnTransport(NewHTTPTransport(...), timeout)。
func NewHTTPClient(timeout time.Duration, skipVerify bool) *http.Client {
	return NewHTTPClientOnTransport(NewHTTPTransport(skipVerify, false), timeout)
}

// NewHTTPTransport 构建与 NewHTTPClient 同源的 Transport。
// proxyFromEnvironment 为 true 时恢复原生客户端的代理行为（HTTP_PROXY / HTTPS_PROXY / NO_PROXY 及其小写形式）。
func NewHTTPTransport(skipVerify, proxyFromEnvironment bool) *http.Transport {
	transport := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: skipVerify},
		ForceAttemptHTTP2: true,
	}
	if proxyFromEnvironment {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return transport
}

// NewHTTPClientOnTransport 用给定的 Transport 构建客户端（单个 Transport 可在多个超时不同的客户端间共享连接池）。
func NewHTTPClientOnTransport(transport *http.Transport, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("重定向次数过多（上限 %d）", maxRedirects)
			}
			return nil
		},
	}
}

// NewClientForURL 按目标地址的证书策略构建客户端
func NewClientForURL(rawURL string, timeout time.Duration) *http.Client {
	return NewHTTPClient(timeout, ShouldSkipVerify(rawURL))
}

// Open 发起 GET 并校验 2xx 状态；调用方负责关闭响应体并限制读取大小
func Open(rawURL string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := NewClientForURL(rawURL, timeout).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// CopyLimited 将 src 复制到 dst，超过 maxBytes 时明确报错（不会静默截断）
func CopyLimited(dst io.Writer, src io.Reader, maxBytes int64) error {
	n, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return fmt.Errorf("资源过大，超过 %dMB 限制", maxBytes/(1024*1024))
	}
	return nil
}
