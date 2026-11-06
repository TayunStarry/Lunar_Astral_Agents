package MutualForum

// account.go 账号申领与轮次生命周期编排。
//
// 月华的论坛身份不靠人工注册：首次调用时 POST /auth/enroll 领取 uid 与一次性
// api_key，随后把凭证与轮次状态写入独立的账号文件，后续调用直接复用。
// 配置文件里也允许直接填 api_key（适合固定部署或迁移已有身份）。
//
// 账号文件的磁盘格式、v2 幂等迁移、.bak 回滚副本与写互斥均由 engine/forum 负责；
// 本文件只保留申领口径与凭证复验：
//   - 申领只发送 name / join_code / qq / uid，不再发送 model / client_version / meta；
//   - 账号文件升级到 schema_version=2，移除 model，新增 qq_masked / avatar_* / kind / test_mode；
//   - 每次覆盖账号文件前都会写 .bak，回滚只需把 .bak 复制回原路径（由 engine/forum 实现）。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	engineforum "LunarSubsystem/LunarHarness/engine/forum"
)

// qqPattern QQ 号：5–12 位纯数字且首位不为 0。
var qqPattern = regexp.MustCompile(`^[1-9]\d{4,11}$`)

// loadAccount 读取本地账号文件；文件不存在或地址已变化时返回 nil。
// 旧结构的幂等迁移由 engine/forum 在同一次读取中完成。
func loadAccount(cfg Config) (*Account, error) {
	return engineforum.LoadAccount(cfg.AccountPath(), cfg.Endpoint())
}

// saveAccount 写入账号文件（原子落盘、.bak 回滚副本与写互斥由 engine/forum 保证）。
func saveAccount(cfg Config, acct *Account) error {
	return engineforum.SaveAccount(cfg.AccountPath(), acct)
}

// enrollAccount 申领一个新身份并返回账号状态；existing 非空时用于 uid 幂等恢复。
func enrollAccount(ctx context.Context, c *Client, cfg Config, existing *Account) (*Account, error) {
	name, generated, nameErr := chooseName(cfg)
	if nameErr != nil {
		return nil, nameErr
	}
	uid := strings.TrimSpace(cfg.UID)
	if uid == "" && existing != nil {
		uid = strings.TrimSpace(existing.InstanceUID)
	}
	if existing != nil && strings.TrimSpace(existing.Name) != "" {
		// 命中既有 uid 时服务端会保留旧昵称；仍按文档携带 name 字段。
		name = existing.Name
		generated = existing.NameGenerated
	}
	if utf8.RuneCountInString(name) > enrollNameMaxRunes {
		name = string([]rune(name)[:enrollNameMaxRunes])
		generated = true
	}
	qq := strings.TrimSpace(cfg.QQ)
	if qq != "" && !qqPattern.MatchString(qq) {
		return nil, ErrQQInvalid
	}

	request := engineforum.EnrollRequest{
		Name:     name,
		JoinCode: strings.TrimSpace(cfg.JoinCode),
		QQ:       qq,
		UID:      uid,
	}
	result, err := c.Enroll(ctx, request)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.APIKey) == "" {
		if result.Idempotent {
			return nil, fmt.Errorf("%w（identity=%s，uid=%s）；请恢复本地 forum_account.json，或在配置里填写原来的 api_key",
				ErrCredentialsLost, result.Identity, result.Instance.UID)
		}
		return nil, errors.New("论坛申领成功但没有返回 api_key")
	}

	now := time.Now().Unix()
	acct := &Account{
		SchemaVersion:  engineforum.AccountSchemaVersion,
		Endpoint:       c.Endpoint,
		InstanceUID:    result.Instance.UID,
		Name:           result.Instance.Name,
		NameGenerated:  generated,
		QQMAsked:       result.Instance.QQMAsked,
		AvatarURL:      result.Instance.AvatarURL,
		AvatarFallback: result.Instance.AvatarFallback,
		Kind:           result.Instance.Kind,
		TestMode:       result.TestMode,
		APIKey:         result.APIKey,
		MinInterval:    result.Poll.MinIntervalSeconds,
		RoundTTL:       result.Poll.RoundTTLSeconds,
		ValidatedAt:    now,
	}
	c.SetAPIKey(result.APIKey)
	logger.Info("LunarMatrix", "已申领论坛身份 %s（%s）", result.Instance.UID, result.Instance.Name)
	return acct, nil
}

// EnsureAccount 返回可用身份：优先使用配置里的 api_key，其次本地账号文件，
// 都没有时自动申领；已有凭证按 TTL 复验，避免每次工具调用都请求 /me。
func EnsureAccount(ctx context.Context, cfg Config, c *Client) (*Account, error) {
	return ensureAccount(ctx, cfg, c, false)
}

// ensureAccount 是 EnsureAccount 的实现；writeLocked 表示调用方已持有论坛写锁，
// 此时申领直接执行，不能再次获取同一把锁。
func ensureAccount(ctx context.Context, cfg Config, c *Client, writeLocked bool) (*Account, error) {
	acct, err := loadAccount(cfg)
	if err != nil {
		return nil, err
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		if acct == nil {
			acct = &Account{SchemaVersion: engineforum.AccountSchemaVersion, Endpoint: c.Endpoint}
		}
		acct.APIKey = key
	}
	if acct == nil || strings.TrimSpace(acct.APIKey) == "" {
		return enrollWithWriteLock(ctx, cfg, c, acct, writeLocked)
	}

	c.SetAPIKey(acct.APIKey)
	now := time.Now().Unix()
	if acct.ValidatedAt <= 0 || now-acct.ValidatedAt > int64(accountValidationTTL/time.Second) {
		me, meErr := c.Me(ctx)
		if meErr != nil {
			var apiErr *APIError
			if errors.As(meErr, &apiErr) && apiErr.Status == http.StatusUnauthorized {
				return nil, &APIError{
					Status:  http.StatusUnauthorized,
					Code:    "unauthorized",
					Message: "互助论坛凭证已失效，请恢复 forum_account.json.bak 后重新申领，或在配置里填写新的 api_key",
				}
			}
			return nil, meErr
		}
		applyMe(acct, me)
		acct.ValidatedAt = now
		if saveErr := saveAccount(cfg, acct); saveErr != nil {
			logger.Warn("LunarMatrix", "保存账号复验状态失败: %v", saveErr)
		}
	}
	return acct, nil
}

// enrollWithWriteLock 负责申领并落盘。读路径触发申领时进入唯一写锁并重新读取账号，
// 避免两个并发只读调用各自领取一个身份；写路径已持锁时直接执行。
func enrollWithWriteLock(ctx context.Context, cfg Config, c *Client, existing *Account, writeLocked bool) (*Account, error) {
	if writeLocked {
		return enrollAndSave(ctx, cfg, c, existing)
	}
	return engineforum.WithWrite(ctx, func() (*Account, error) {
		fresh, err := loadAccount(cfg)
		if err != nil {
			return nil, err
		}
		if fresh != nil && strings.TrimSpace(fresh.APIKey) != "" {
			c.SetAPIKey(fresh.APIKey)
			return fresh, nil
		}
		if fresh == nil {
			fresh = existing
		}
		return enrollAndSave(ctx, cfg, c, fresh)
	})
}

// enrollAndSave 申领身份并写入账号文件。
func enrollAndSave(ctx context.Context, cfg Config, c *Client, existing *Account) (*Account, error) {
	acct, err := enrollAccount(ctx, c, cfg, existing)
	if err != nil {
		return nil, err
	}
	if err := saveAccount(cfg, acct); err != nil {
		return nil, err
	}
	return acct, nil
}

// applyMe 把 /me 的最新公开信息同步到本地账号。
func applyMe(acct *Account, me *Me) {
	acct.InstanceUID = me.Instance.UID
	acct.Name = me.Instance.Name
	acct.QQMAsked = me.Instance.QQMAsked
	acct.AvatarURL = me.Instance.AvatarURL
	acct.AvatarFallback = me.Instance.AvatarFallback
	acct.Kind = me.Instance.Kind
	if me.Limits.TestExempt {
		acct.TestMode = true
	}
	acct.RoundID = me.Round.RoundID
	acct.RoundUsed = me.Round.Used
	roundIdleReset(acct, me)
	if me.Round.TTLSeconds > 0 {
		acct.RoundTTL = me.Round.TTLSeconds
	}
	if me.Round.PollMinIntervalS > 0 {
		acct.MinInterval = me.Round.PollMinIntervalS
	}
	if me.Stats.LastPollAt != nil {
		acct.LastPollAt = parseISOTime(*me.Stats.LastPollAt)
	}
}

// roundIdleReset 只在轮次未被使用且能解析出签发时刻时保留复用窗口。
func roundIdleReset(acct *Account, me *Me) {
	acct.RoundIssuedAt = 0
	if me.Round.Used || me.Round.IssuedAt == nil {
		return
	}
	acct.RoundIssuedAt = parseISOTime(*me.Round.IssuedAt)
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
		NameGenerated:   acct.NameGenerated,
		QQMAsked:        acct.QQMAsked,
		AvatarURL:       acct.AvatarURL,
		AvatarFallback:  acct.AvatarFallback,
		Kind:            acct.Kind,
		TestMode:        acct.TestMode,
		RoundID:         acct.RoundID,
		RoundUsed:       acct.RoundUsed,
		RoundIssuedAt:   acct.RoundIssuedAt,
		LastSeq:         acct.LastSeq,
		EventSeq:        acct.EventSeq,
		RoundTTLSeconds: acct.RoundTTL,
	}
}

// parseISOTime 解析论坛返回的 ISO 时间（兼容带小数秒与 +08:00）；失败返回 0。
func parseISOTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}
