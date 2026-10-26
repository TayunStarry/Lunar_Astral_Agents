package lunarmatrix

// client.go 论坛 API 的 HTTP 基础层：统一信封解析、错误映射与请求构造。

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// newClient 按配置构造客户端（不带凭证；凭证由 EnsureAccount 填充）。
func newClient(cfg Config) *Client {
	return &Client{
		endpoint: cfg.Endpoint(),
		http:     &http.Client{Timeout: cfg.RequestTimeout()},
	}
}

// request 发送一次 API 请求并解析统一信封。
func (c *Client) request(ctx context.Context, method, path string, query url.Values, body any, auth bool, out any) (*apiMeta, error) {
	if c.endpoint == "" {
		return nil, ErrInvalidBaseURL
	}
	target := c.endpoint + "/" + strings.TrimLeft(path, "/")
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求失败: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		if c.apiKey == "" {
			return nil, &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "缺少互助论坛访问凭证"}
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求论坛失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("读取论坛响应失败: %w", err)
	}

	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return nil, &APIError{
				Status:  resp.StatusCode,
				Code:    "not_found_html",
				Message: "论坛接口返回 404，响应体不是 JSON（可能被前置服务器改写）",
				Details: map[string]any{"body": truncateText(string(raw), 160)},
			}
		}
		return nil, &APIError{
			Status:  resp.StatusCode,
			Code:    "bad_response",
			Message: "论坛接口响应不是合法 JSON",
			Details: map[string]any{"body": truncateText(string(raw), 160)},
		}
	}

	if resp.StatusCode >= 400 || !envelope.OK {
		apiErr := &APIError{
			Status:  resp.StatusCode,
			Code:    "http_" + strconv.Itoa(resp.StatusCode),
			Message: "论坛接口请求失败",
		}
		if envelope.Error != nil {
			apiErr.Code = envelope.Error.Code
			apiErr.Message = envelope.Error.Message
			apiErr.Details = envelope.Error.Details
		}
		if retry, parseErr := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); parseErr == nil {
			apiErr.RetryAfter = retry
		}
		return nil, apiErr
	}

	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return nil, fmt.Errorf("解析论坛响应失败: %w", err)
		}
	}
	return &envelope.Meta, nil
}

// Health 无认证健康检查。
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var health Health
	if _, err := c.request(ctx, http.MethodGet, "health", nil, nil, false, &health); err != nil {
		return nil, err
	}
	return &health, nil
}

// Me 读取当前身份、统计与轮次状态。
func (c *Client) Me(ctx context.Context) (*Me, error) {
	var me Me
	if _, err := c.request(ctx, http.MethodGet, "me", nil, nil, true, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// newRequestID 生成可读的幂等键（client_request_id）。
func newRequestID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}

// truncateText 按字符截断，避免错误详情过长。
func truncateText(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "…"
}
