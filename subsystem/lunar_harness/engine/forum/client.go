package forum

// client.go 论坛 HTTP 传输层：统一信封、鉴权头、错误映射与瞬态重试。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/internal/textutil"
)

// New 创建论坛 HTTP 客户端。
func New(endpoint string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	return &Client{
		Endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		http:     &http.Client{Timeout: timeout},
	}
}

// SetAPIKey 设置 Bearer 鉴权 key。
func (c *Client) SetAPIKey(key string) {
	c.apiKey = strings.TrimSpace(key)
}

// HasAPIKey 判断是否已设置鉴权 key。
func (c *Client) HasAPIKey() bool {
	return c.apiKey != ""
}

// NewRequestID 生成幂等键。
func NewRequestID(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "req"
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(buf)
}

// Health 读取服务健康状态（无需鉴权）。
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var out Health
	if _, err := c.request(ctx, http.MethodGet, "/health", nil, nil, false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Me 读取当前身份与轮次（需要鉴权）。
func (c *Client) Me(ctx context.Context) (*Me, error) {
	var out Me
	if _, err := c.request(ctx, http.MethodGet, "/me", nil, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// request 统一发送请求；forum_deploying 仅对无请求体的 GET 做有限重试，
// 避免对写接口重复提交。
func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any, auth bool, out any) (*apiMeta, error) {
	if strings.TrimSpace(c.Endpoint) == "" {
		return nil, ErrEndpointMissing
	}
	if auth && c.apiKey == "" {
		return nil, ErrMissingAPIKey
	}

	var payload []byte
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}
		payload = raw
	}

	canRetry := body == nil && (method == http.MethodGet || method == http.MethodHead)
	for attempt := 0; ; attempt++ {
		meta, err := c.doRequest(ctx, method, path, query, payload, auth, out)
		if err == nil {
			return meta, nil
		}
		if !canRetry || attempt >= maxTransientRetries {
			return nil, err
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "forum_deploying" {
			return nil, err
		}
		wait := apiErr.RetryAfter
		if wait <= 0 {
			wait = time.Second
		}
		if wait > 10*time.Second {
			wait = 10 * time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// doRequest 执行单次请求并解析统一信封。
func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, payload []byte, auth bool, out any) (*apiMeta, error) {
	requestURL := c.Endpoint + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, reader)
	if err != nil {
		return nil, fmt.Errorf("构造论坛请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求互助论坛失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取互助论坛响应失败: %w", err)
	}
	if int64(len(raw)) > maxResponseBytes {
		return nil, fmt.Errorf("互助论坛响应体超过 %d 字节", maxResponseBytes)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, fmt.Errorf("%w（HTTP %d）", ErrEmptyResponse, resp.StatusCode)
	}

	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		code := "non_json"
		if resp.StatusCode == http.StatusNotFound {
			code = "not_found_html"
		}
		return nil, &APIError{
			Code:   code,
			Status: resp.StatusCode,
			Message: fmt.Sprintf("论坛返回非 JSON 响应（HTTP %d）：%s",
				resp.StatusCode, textutil.TruncateRunes(strings.TrimSpace(string(raw)), errorTextRunes)),
		}
	}

	if env.Error != nil {
		return nil, buildAPIError(env, resp)
	}
	if !env.OK {
		return nil, &APIError{
			Code:    "not_ok",
			Status:  resp.StatusCode,
			Message: "论坛返回了 ok=false 但缺少 error 字段",
		}
	}
	if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return nil, fmt.Errorf("解析论坛响应失败: %w", err)
		}
	}
	return &env.Meta, nil
}

// buildAPIError 把服务端错误信封转成 APIError。
func buildAPIError(env apiEnvelope, resp *http.Response) *APIError {
	body := env.Error
	apiErr := &APIError{
		Code:      strings.TrimSpace(body.Code),
		Message:   strings.TrimSpace(body.Message),
		Status:    resp.StatusCode,
		RequestID: env.Meta.RequestID,
		Details:   body.Details,
		Allow:     resp.Header.Get("Allow"),
	}
	if apiErr.Code == "" {
		apiErr.Code = "unknown_error"
	}
	if apiErr.Message == "" {
		apiErr.Message = fmt.Sprintf("论坛请求失败（HTTP %d，%s）", resp.StatusCode, apiErr.Code)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		apiErr.Message += "（前置服务器未放行该方法，请检查是否禁止了 PATCH/DELETE）"
	}
	apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	if apiErr.RetryAfter <= 0 {
		apiErr.RetryAfter = retryAfterFromDetails(apiErr.Details)
	}
	return apiErr
}

// parseRetryAfter 支持秒数或 HTTP 日期格式。
func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return 0
}

// retryAfterFromDetails 读取 details.retry_after_seconds 作为兜底。
func retryAfterFromDetails(details map[string]any) time.Duration {
	if len(details) == 0 {
		return 0
	}
	value, ok := details["retry_after_seconds"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return time.Duration(typed * float64(time.Second))
		}
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Second
		}
	case json.Number:
		if seconds, err := typed.Int64(); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 0
}
