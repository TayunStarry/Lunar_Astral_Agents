package ltp9

// ==== 熔断器（self-contained 移植 YaraFlow circuit_breaker） ====
// 模型垄断下：单一模型（agent 对话模型）对应一个熔断器；closed/open/half-open + 滑动窗口失败计数。

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// errCircuitOpen 熔断器已打开，请求被拒绝。
var errCircuitOpen = errors.New("circuit breaker is open, request rejected")

// defaultCircuitConfig 默认熔断器配置。
func defaultCircuitConfig() ltmCircuitConfig {
	return ltmCircuitConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		WindowDuration:   60 * time.Second,
		OpenDuration:     30 * time.Second,
		HalfOpenMaxReqs:  1,
	}
}

// newCircuitBreaker 按默认配置创建熔断器。cfg 为零值时套用默认值。
func newCircuitBreaker(name string, cfg ltmCircuitConfig) *ltmCircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 60 * time.Second
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 30 * time.Second
	}
	if cfg.HalfOpenMaxReqs <= 0 {
		cfg.HalfOpenMaxReqs = 1
	}
	cb := &ltmCircuitBreaker{
		name:          name,
		config:        cfg,
		state:         ltmCircuitClosed,
		stateChangedAt: time.Now(),
	}
	return cb
}

// currentState 返回当前状态（atomic 读）。
func (cb *ltmCircuitBreaker) currentState() ltmCircuitState {
	return ltmCircuitState(atomic.LoadInt32((*int32)(&cb.state)))
}

// setState 设置状态（需持有 cb.mu）。
func (cb *ltmCircuitBreaker) setState(s ltmCircuitState) {
	old := cb.currentState()
	atomic.StoreInt32((*int32)(&cb.state), int32(s))
	cb.stateChangedAt = time.Now()
	if old != s {
		LoggerGeneral.Info(ServiceName, "[模型熔断器] 状态变化 breaker=%s from=%s to=%s", cb.name, old, s)
	}
}

// allow 返回是否允许请求通过。
func (cb *ltmCircuitBreaker) allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.currentState() {
	case ltmCircuitClosed:
		cb.pruneFailures()
		if len(cb.failures) >= cb.config.FailureThreshold {
			cb.setState(ltmCircuitOpen)
			return errCircuitOpen
		}
		return nil
	case ltmCircuitOpen:
		if time.Since(cb.lastFailTime) >= cb.config.OpenDuration {
			cb.setState(ltmCircuitHalfOpen)
			cb.halfOpenReqs = 0
			cb.halfOpenPassed = 0
			if int(cb.halfOpenReqs) < cb.config.HalfOpenMaxReqs {
				atomic.AddInt32(&cb.halfOpenReqs, 1)
				return nil
			}
			return errCircuitOpen
		}
		return errCircuitOpen
	case ltmCircuitHalfOpen:
		if int(atomic.LoadInt32(&cb.halfOpenReqs)) >= cb.config.HalfOpenMaxReqs {
			return errCircuitOpen
		}
		atomic.AddInt32(&cb.halfOpenReqs, 1)
		return nil
	default:
		return errCircuitOpen
	}
}

// recordSuccess 记录一次成功调用。
func (cb *ltmCircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.currentState() {
	case ltmCircuitClosed:
		cb.failures = nil
	case ltmCircuitHalfOpen:
		passed := atomic.AddInt32(&cb.halfOpenPassed, 1)
		if int(passed) >= cb.config.SuccessThreshold {
			cb.failures = nil
			cb.setState(ltmCircuitClosed)
		}
	case ltmCircuitOpen:
		cb.failures = nil
		cb.setState(ltmCircuitClosed)
	}
}

// recordFailure 记录一次失败（计入熔断阈值）。
// retryable = true（可重试错误，如 429 限流）不计入滑动窗口失败计数。
func (cb *ltmCircuitBreaker) recordFailure(retryable bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.lastFailTime = time.Now()
	if retryable {
		return
	}
	cb.pruneFailures()
	switch cb.currentState() {
	case ltmCircuitClosed:
		cb.failures = append(cb.failures, time.Now())
		if len(cb.failures) >= cb.config.FailureThreshold {
			cb.setState(ltmCircuitOpen)
		}
	case ltmCircuitHalfOpen:
		cb.failures = append(cb.failures, time.Now())
		cb.setState(ltmCircuitOpen)
	case ltmCircuitOpen:
		cb.failures = append(cb.failures, time.Now())
	}
}

// pruneFailures 清理滑动窗口外的失败记录（需持有 cb.mu）。
func (cb *ltmCircuitBreaker) pruneFailures() {
	cutoff := time.Now().Add(-cb.config.WindowDuration)
	valid := cb.failures[:0]
	for _, t := range cb.failures {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	cb.failures = valid
}

// status 返回熔断器状态摘要（供查询）。
func (cb *ltmCircuitBreaker) status() map[string]any {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.pruneFailures()
	return map[string]any{
		"name":              cb.name,
		"state":             cb.currentState().String(),
		"failure_count":     len(cb.failures),
		"failure_threshold": cb.config.FailureThreshold,
		"open_duration":     cb.config.OpenDuration.String(),
		"window_duration":   cb.config.WindowDuration.String(),
	}
}

// reset 手动重置熔断器。
func (cb *ltmCircuitBreaker) reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = nil
	cb.halfOpenReqs = 0
	cb.halfOpenPassed = 0
	cb.setState(ltmCircuitClosed)
}

// ==== 熔断器注册表（模型垄断下收敛到单一 agent 模型） ====

var circuitRegistry sync.Map // key: model name, value: *ltmCircuitBreaker

// getCircuitBreaker 获取（或惰性创建）指定模型的熔断器。
func getCircuitBreaker(modelName string) *ltmCircuitBreaker {
	if v, ok := circuitRegistry.Load(modelName); ok {
		return v.(*ltmCircuitBreaker)
	}
	cb := newCircuitBreaker(modelName, defaultCircuitConfig())
	actual, _ := circuitRegistry.LoadOrStore(modelName, cb)
	return actual.(*ltmCircuitBreaker)
}

// getAllCircuitBreakers 返回全部熔断器状态快照。
func getAllCircuitBreakers() []map[string]any {
	out := []map[string]any{}
	circuitRegistry.Range(func(_, v any) bool {
		out = append(out, v.(*ltmCircuitBreaker).status())
		return true
	})
	return out
}

// resetCircuitBreaker 手动重置指定模型熔断器。
func resetCircuitBreaker(modelName string) {
	if v, ok := circuitRegistry.Load(modelName); ok {
		v.(*ltmCircuitBreaker).reset()
	}
}

// isRetryableError 判断是否为可重试错误（429 限流等，不计入熔断失败计数）。
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "throttl") ||
		strings.Contains(msg, "too many requests")
}