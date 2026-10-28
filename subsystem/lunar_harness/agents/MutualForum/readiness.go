package MutualForum

// readiness.go 条件暴露的核心：论坛真的能访问时才让工具可见。
//
// BridgeReady() 会被工具列表构建逻辑在每一轮对话前调用，因此这里必须足够轻：
//   - 配置开关/地址无效：立即 false；
//   - 健康探测按配置 TTL 缓存，过期后才重新请求；
//   - 并发调用时只有一个探测在飞行，其余先用上一次结果。

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Ready 返回工具是否应暴露给月华。
func BridgeReady() bool {
	cfg, err := LoadConfig()
	if err != nil || cfg.Validate() != nil {
		return false
	}
	endpoint := cfg.Endpoint()

	probeMu.Lock()
	sameEndpoint := probeCache.Endpoint == endpoint
	if sameEndpoint && time.Since(probeCache.CheckedAt) < cfg.HealthCache() {
		ok := probeCache.OK
		probeMu.Unlock()
		return ok
	}
	if sameEndpoint && !probeCache.CheckedAt.IsZero() {
		// 已经探测过：过期后后台刷新，本次先用上次结果，避免每轮对话都阻塞。
		if !probeRunning {
			probeRunning = true
			go probeOnce(cfg)
		}
		ok := probeCache.OK
		probeMu.Unlock()
		return ok
	}
	if probeRunning {
		probeMu.Unlock()
		return false
	}
	probeRunning = true
	probeMu.Unlock()

	// 首次探测同步执行：工具是否可见从第一轮起就有确定答案。
	health, probeErr := probeOnce(cfg)
	return probeErr == nil && health != nil && health.Enabled && identityOrEnrollable(cfg, health)
}

// Refresh 强制刷新健康探测（agent 在真正执行操作前调用，避免用旧缓存误判）。
func Refresh(ctx context.Context) (*Health, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return probeOnceContext(ctx, cfg)
}

// probeOnce 用后台默认超时探测一次并写入缓存。
func probeOnce(cfg Config) (*Health, error) {
	return probeOnceContext(context.Background(), cfg)
}

// probeOnceContext 执行一次探测并更新缓存；OK 表示「健康 + 有身份或可申领」。
func probeOnceContext(ctx context.Context, cfg Config) (*Health, error) {
	health, probeErr := probeHealth(ctx, cfg)

	probeMu.Lock()
	probeCache = readinessProbe{
		Endpoint:  cfg.Endpoint(),
		OK:        probeErr == nil && health != nil && health.Enabled && identityOrEnrollable(cfg, health),
		Health:    healthOrZero(health),
		Error:     errorText(probeErr),
		CheckedAt: time.Now(),
	}
	probeRunning = false
	probeMu.Unlock()
	return health, probeErr
}

// InspectStatus 汇总健康、身份与统计，供 forum_status 工具使用。
func InspectStatus(ctx context.Context) (*Status, error) {
	cfg, err := LoadConfig()
	if err != nil {
		status := &Status{Error: err.Error()}
		if errors.Is(err, ErrConfigMissing) {
			status.BaseURL = ""
		}
		return status, nil
	}
	status := &Status{Enabled: cfg.Enabled, BaseURL: cfg.SiteURL()}
	if err := cfg.Validate(); err != nil {
		status.Error = err.Error()
		return status, nil
	}

	probeCtx, cancel := context.WithTimeout(ctx, cfg.HealthTimeout())
	health, healthErr := probeHealth(probeCtx, cfg)
	cancel()
	if healthErr != nil {
		status.Error = healthErr.Error()
		return status, nil
	}
	status.Health = health
	status.Reachable = health.Enabled

	acct, acctErr := loadAccount(cfg)
	if acctErr != nil {
		status.IdentityError = acctErr.Error()
	} else if acct != nil {
		status.Identity = accountView(acct)
		client := newClient(cfg)
		client.apiKey = acct.APIKey
		meCtx, meCancel := context.WithTimeout(ctx, cfg.RequestTimeout())
		me, meErr := client.Me(meCtx)
		meCancel()
		if meErr != nil {
			status.IdentityError = meErr.Error()
		} else {
			status.Me = me
		}
	}
	return status, nil
}

// probeHealth 发起一次健康探测。
func probeHealth(ctx context.Context, cfg Config) (*Health, error) {
	client := newClient(cfg)
	probeCtx, cancel := context.WithTimeout(ctx, cfg.HealthTimeout())
	defer cancel()
	health, err := client.Health(probeCtx)
	if err != nil {
		return nil, fmt.Errorf("互助论坛健康检查失败: %w", err)
	}
	if !health.Enabled {
		return health, errors.New("互助论坛当前未开放")
	}
	return health, nil
}

// identityOrEnrollable 有凭证，或论坛仍开放申领时，工具才值得暴露。
func identityOrEnrollable(cfg Config, health *Health) bool {
	if health == nil {
		return false
	}
	if hasIdentity(cfg) {
		return true
	}
	return health.EnrollMode != "closed"
}

// hasIdentity 检查配置或账号文件里是否已有凭证。
func hasIdentity(cfg Config) bool {
	if cfg.APIKey != "" {
		return true
	}
	acct, err := loadAccount(cfg)
	return err == nil && acct != nil && acct.APIKey != ""
}

func healthOrZero(health *Health) Health {
	if health == nil {
		return Health{}
	}
	return *health
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
