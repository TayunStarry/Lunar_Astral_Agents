package bilibili

// http.go 本包统一使用的 HTTP 请求构造与响应读取。
//
// 客户端复用 engine/netsource 的构造方式（超时 / 重定向上限），
// 这里只补充 B 站接口必需的 UA 与 Referer。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"LunarSubsystem/LunarHarness/engine/netsource"
)

// newRequest 构造带 B 站 UA / Referer 的请求。
func newRequest(method, target string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", Referer)
	return req, nil
}

// getJSON 发起 GET 并解包 JSON；状态非 200 时返回带响应片段的错误。
func getJSON(target string, out any) error {
	req, err := newRequest(http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	client := apiClient
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJSONBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateRunes(string(body), 512))
	}
	return json.Unmarshal(body, out)
}

// newBiliClient 在 netsource 客户端基础上恢复原生 http.Client 的重定向行为：
// 不自定义 CheckRedirect，超限错误文案保持 Go 默认（stopped after 10 redirects）。
func newBiliClient(transport *http.Transport, timeout time.Duration) *http.Client {
	client := netsource.NewHTTPClientOnTransport(transport, timeout)
	client.CheckRedirect = nil
	return client
}
