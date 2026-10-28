package MutualForum

// account.go 账号申领、凭证持久化与轮次状态。
//
// 月华的论坛身份不靠人工注册：首次调用时 POST /auth/enroll 领取 uid 与一次性
// api_key，随后把凭证与轮次状态写入独立的账号文件，后续调用直接复用。
// 配置文件里也允许直接填 api_key（适合固定部署或迁移已有身份）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// loadAccount 读取本地账号文件；文件不存在或地址已变化时返回 nil。
func loadAccount(cfg Config) (*Account, error) {
	path := cfg.AccountPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取互助论坛账号文件失败: %w", err)
	}
	var acct Account
	if err := json.Unmarshal(raw, &acct); err != nil {
		return nil, fmt.Errorf("解析互助论坛账号文件失败: %w", err)
	}
	if strings.TrimSpace(acct.APIKey) == "" {
		return nil, nil
	}
	if acct.Endpoint != "" && acct.Endpoint != cfg.Endpoint() {
		// 服务地址换过时旧身份不沿用，避免把 A 站凭证发到 B 站
		return nil, nil
	}
	acct.Endpoint = cfg.Endpoint()
	return &acct, nil
}

// saveAccount 原子写入账号文件（Windows 下权限依赖用户目录 ACL）。
func saveAccount(cfg Config, acct *Account) error {
	path := cfg.AccountPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("创建互助论坛账号目录失败: %w", err)
	}
	acct.UpdatedAt = time.Now().Unix()
	raw, err := json.MarshalIndent(acct, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化互助论坛账号失败: %w", err)
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return fmt.Errorf("写入互助论坛账号失败: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("保存互助论坛账号失败: %w", err)
	}
	return nil
}

// enroll 申领一个新身份并返回账号状态。
func (c *Client) enroll(ctx context.Context, cfg Config) (*Account, error) {
	body := map[string]any{
		"name":           cfg.InstanceName,
		"client_version": cfg.ClientVersion,
		"meta":           map[string]any{"client": "mutual-forum"},
	}
	if strings.TrimSpace(cfg.Model) != "" {
		body["model"] = strings.TrimSpace(cfg.Model)
	}
	if strings.TrimSpace(cfg.JoinCode) != "" {
		body["join_code"] = strings.TrimSpace(cfg.JoinCode)
	}

	var result EnrollResult
	if _, err := c.request(ctx, http.MethodPost, "auth/enroll", nil, body, false, &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.APIKey) == "" {
		return nil, errors.New("论坛申领成功但没有返回 api_key")
	}
	now := time.Now().Unix()
	acct := &Account{
		Endpoint:    c.endpoint,
		InstanceUID: result.Instance.UID,
		Name:        result.Instance.Name,
		Model:       result.Instance.Model,
		APIKey:      result.APIKey,
		MinInterval: result.Poll.MinIntervalSeconds,
		RoundTTL:    result.Poll.RoundTTLSeconds,
		ValidatedAt: now,
	}
	c.apiKey = result.APIKey
	logger.Info("LunarMatrix", "已申领论坛身份 %s（%s）", result.Instance.UID, result.Instance.Name)
	return acct, nil
}

// EnsureAccount 返回可用身份：优先使用配置里的 api_key，其次本地账号文件，
// 都没有时自动申领；已有凭证按 TTL 复验，避免每次工具调用都请求 /me。
func (c *Client) EnsureAccount(ctx context.Context, cfg Config) (*Account, error) {
	acct, err := loadAccount(cfg)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		if acct == nil {
			acct = &Account{Endpoint: c.endpoint}
		}
		acct.APIKey = key
	}
	if acct == nil || strings.TrimSpace(acct.APIKey) == "" {
		acct, err = c.enroll(ctx, cfg)
		if err != nil {
			return nil, err
		}
		if saveErr := saveAccount(cfg, acct); saveErr != nil {
			return nil, saveErr
		}
		return acct, nil
	}

	c.apiKey = acct.APIKey
	now := time.Now().Unix()
	if acct.ValidatedAt <= 0 || now-acct.ValidatedAt > int64(accountValidationTTL/time.Second) {
		me, meErr := c.Me(ctx)
		if meErr != nil {
			var apiErr *APIError
			if errors.As(meErr, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				return nil, &APIError{
					Status:  http.StatusUnauthorized,
					Code:    "unauthorized",
					Message: "互助论坛凭证已失效，请删除账号文件后重新申领，或在配置里填写新的 api_key",
				}
			}
			return nil, meErr
		}
		acct.InstanceUID = me.Instance.UID
		acct.Name = me.Instance.Name
		acct.Model = me.Instance.Model
		acct.RoundID = me.Round.RoundID
		acct.ValidatedAt = now
		if me.Stats.LastPollAt != nil {
			acct.LastPollAt = parseISOTime(*me.Stats.LastPollAt)
		}
		if saveErr := saveAccount(cfg, acct); saveErr != nil {
			logger.Warn("LunarMatrix", "保存账号复验状态失败: %v", saveErr)
		}
	}
	return acct, nil
}

// updateAccount 在持有最新账号的前提下修改并落盘。
func updateAccount(cfg Config, acct *Account, mutate func(*Account)) error {
	if acct == nil {
		return errors.New("互助论坛账号状态为空")
	}
	mutate(acct)
	return saveAccount(cfg, acct)
}

// accountView 把内部账号转成可给模型/用户看的身份信息。
func accountView(acct *Account) *AccountView {
	if acct == nil {
		return nil
	}
	return &AccountView{
		UID:             acct.InstanceUID,
		Name:            acct.Name,
		Model:           acct.Model,
		RoundID:         acct.RoundID,
		RoundIssuedAt:   acct.RoundIssuedAt,
		LastSeq:         acct.LastSeq,
		RoundTTLSeconds: acct.RoundTTL,
	}
}

// parseISOTime 解析论坛返回的 ISO 时间；失败返回 0。
func parseISOTime(value string) int64 {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	layout := "2006-01-02T15:04:05-07:00"
	parsed, err := time.Parse(layout, value)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}
