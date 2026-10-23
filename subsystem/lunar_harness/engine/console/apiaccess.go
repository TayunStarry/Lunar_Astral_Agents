package console

// ==== 接口访问的会话级策略：令牌记忆 / 限流冷却 ====
//
// 背景（用户实测）：配置里没填 `release.gitee_token` 时，所有列表请求都走 Gitee 的
// **匿名额度**（按 IP 计、极低），很快就被打满，返回
// `403 Forbidden (Rate Limit Exceeded)` —— 表现为「应用市场与更新包列表都拉不到」。
//
// 三条对策：
//  1. **令牌记忆**：用户在发布/安装流程里输入过的令牌，本次会话内所有请求都带上
//     （配置里有则以配置为准），额度从匿名档升到认证档；
//  2. **限流冷却**：一旦吃到期限流，短时间内不再打接口，直接给出人话提示，
//     避免用户反复点刷新把额度耗得更久；
//  3. **列表缓存**：同一份列表在 TTL 内复用，进入屏幕不再重复请求（手动刷新除外）。

import (
	"fmt"
	"sync"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/channel"
	"LunarSubsystem/LunarHarness/engine/console/ltpxmarket"
	"LunarSubsystem/LunarHarness/engine/console/updatepack"
)

// listCacheTTL 列表结果复用时长：自动进入屏幕时用缓存，手动刷新（r / 拉取）总是重新请求
const listCacheTTL = 60 * time.Second

// rateLimitCooldown 吃到期限流后的静默期
const rateLimitCooldown = 60 * time.Second

var (
	apiStateMu       sync.Mutex
	sessionToken     string    // 用户在本次会话里输入过的 Gitee 令牌
	rateLimitedUntil time.Time // 限流冷却截止时刻
)

// SetSessionToken 记住本次会话的 Gitee 令牌（空串表示清除）
func SetSessionToken(tok string) {
	apiStateMu.Lock()
	defer apiStateMu.Unlock()
	sessionToken = tok
}

// sessionTokenValue 取当前会话令牌
func sessionTokenValue() string {
	apiStateMu.Lock()
	defer apiStateMu.Unlock()
	return sessionToken
}

// noteAPIRateLimit 记录一次限流，开启冷却
func noteAPIRateLimit(err error) bool {
	if !isRateLimitedErr(err) {
		return false
	}
	apiStateMu.Lock()
	defer apiStateMu.Unlock()
	rateLimitedUntil = time.Now().Add(rateLimitCooldown)
	return true
}

// rateLimitRemaining 冷却剩余时长（0 表示未在冷却）
func rateLimitRemaining() time.Duration {
	apiStateMu.Lock()
	defer apiStateMu.Unlock()
	if time.Now().After(rateLimitedUntil) {
		return 0
	}
	return time.Until(rateLimitedUntil)
}

// clearRateLimit 请求成功时解除冷却
func clearRateLimit() {
	apiStateMu.Lock()
	defer apiStateMu.Unlock()
	rateLimitedUntil = time.Time{}
}

// withSessionTokenUpdate 把会话令牌补进更新包配置（配置里已有则以其为准）
func withSessionTokenUpdate(cfg *updatepack.Config) *updatepack.Config {
	if cfg == nil {
		return nil
	}
	if cfg.GiteeToken == "" {
		cp := *cfg
		cp.GiteeToken = sessionTokenValue()
		return &cp
	}
	return cfg
}

// withSessionTokenMarket 把会话令牌补进市场配置（配置里已有则以其为准）
func withSessionTokenMarket(cfg *ltpxmarket.Config) *ltpxmarket.Config {
	if cfg == nil {
		return nil
	}
	if cfg.GiteeToken == "" {
		cp := *cfg
		cp.GiteeToken = sessionTokenValue()
		return &cp
	}
	return cfg
}

// tokenConfigured 当前请求是否会带上令牌（用于生成提示文案）
func tokenConfigured() bool {
	if sessionTokenValue() != "" {
		return true
	}
	if cfg := GetUpdatePackConfig(); cfg != nil && cfg.GiteeToken != "" {
		return true
	}
	if cfg := GetMarketConfig(); cfg != nil && cfg.GiteeToken != "" {
		return true
	}
	return false
}

// isRateLimitedErr 判定错误是否属于接口限流（统一走 channel 的分类口径）
func isRateLimitedErr(err error) bool {
	return channel.IsRateLimited(err)
}

// apiFriendlyError 生成给用户看的人话提示（自动带上「本次是否带令牌」这一事实）
func apiFriendlyError(err error) string {
	if err == nil {
		return ""
	}
	return channel.FriendlyAPIError(err, tokenConfigured())
}

// shortDuration 时长的紧凑中文写法（用于「N 秒/分钟后可重试」一类提示）
func shortDuration(d time.Duration) string {
	if d <= 0 {
		return "0 秒"
	}
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()+0.5))
	}
	return fmt.Sprintf("%d 分钟", int(d.Minutes()+0.5))
}

// friendlyErr 把错误转成 error（供 withErrStrip 一类需要 error 的渲染入口使用）。
// errString 已在 view_proxy.go 定义，这里复用其类型，保证错误条样式一致。
func friendlyErr(err error) error {
	if err == nil {
		return nil
	}
	if msg := apiFriendlyError(err); msg != "" {
		return errString(msg)
	}
	return err
}
