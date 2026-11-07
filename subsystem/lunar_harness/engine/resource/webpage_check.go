package resource

// webpage_check.go 网页信封的可达性校验。
//
// 只校验「链接是否可达」，不根据响应内容或 URL 形态推断资源类型；
// 类型来自发送侧的 webpage.platform 声明。

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// webpageCheckTimeout 接入链路的网页可达性校验超时
const webpageCheckTimeout = 6 * time.Second

// CheckReachable 校验网页链接是否可达；不可达返回错误（调用方据此判定信封无效）。
func CheckReachable(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("网页链接为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), webpageCheckTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	request.Header.Set("Range", "bytes=0-0")
	client := &http.Client{
		Timeout: webpageCheckTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 1<<10)
	if response.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return nil
}
