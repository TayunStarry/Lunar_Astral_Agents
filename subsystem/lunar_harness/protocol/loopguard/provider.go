package loopguard

// provider.go 默认检测参数的配置提供者：protocol 只消费，不做配置读取。

import "sync"

var (
	// optionsMu 保护默认参数提供者
	optionsMu sync.RWMutex
	// optionsProvider 当前注入的默认参数提供者
	optionsProvider DefaultOptionsProvider
)

// SetDefaultOptionsProvider 注入默认参数提供者（由 engine 装配包调用）。
func SetDefaultOptionsProvider(provider DefaultOptionsProvider) {
	optionsMu.Lock()
	optionsProvider = provider
	optionsMu.Unlock()
}

// configuredOptions 读取当前配置覆盖值。
func configuredOptions() (Options, bool) {
	optionsMu.RLock()
	provider := optionsProvider
	optionsMu.RUnlock()
	if provider == nil {
		return Options{}, false
	}
	return provider(), true
}
