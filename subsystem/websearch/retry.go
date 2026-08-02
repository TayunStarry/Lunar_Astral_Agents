package websearch

import (
	"fmt"
	"math"
	"net/http"
	"time"
)

// shouldRetry 判断是否应该重试：网络错误 或 429/503 状态码
func shouldRetry(statusCode int, err error) bool {
	if err != nil {
		return true
	}
	return statusCode == http.StatusTooManyRequests || statusCode == http.StatusServiceUnavailable
}

// doWithRetry 执行带重试的 HTTP 请求，使用指数退避策略
// 退避公式：base × 2^attempt，最大 5 秒
func doWithRetry(client *http.Client, req *http.Request, cfg HTTPConfig) (*http.Response, error) {
	maxRetries := cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}
	backoff := cfg.RetryBackoff
	if backoff <= 0 {
		backoff = 500 * time.Millisecond
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// 指数退避，最大 5 秒
			delay := time.Duration(math.Min(float64(backoff)*math.Pow(2, float64(attempt-1)), float64(5*time.Second)))
			time.Sleep(delay)
		}

		// 每次重试需要重新构造 body（如果 request body 已被消耗）
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if shouldRetry(resp.StatusCode, nil) {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("重试 %d 次后仍然失败: %w", maxRetries, lastErr)
}