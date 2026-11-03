package agentbase

// provider.go 模型端点提供者：protocol 只消费，不做配置读取与 I/O。
//
// 由 engine 侧装配包在启动时注入；未注入时模型调用会返回明确的未配置错误，
// 避免把“缺少装配”伪装成普通的 HTTP 请求失败。

import "sync"

// ModelEndpoint 一次模型调用所需的端点信息。
type ModelEndpoint struct {
	// URL 模型服务地址（可带 /v1，也可不带；基座会统一补齐）
	URL string
	// Model 模型名（空 = 由服务端默认）
	Model string
}

// ModelEndpointProvider 返回当前模型端点；实现由装配层提供。
type ModelEndpointProvider func() ModelEndpoint

var (
	// endpointMu 保护端点提供者
	endpointMu sync.RWMutex
	// endpointProvider 当前注入的端点提供者
	endpointProvider ModelEndpointProvider
)

// SetModelEndpointProvider 注入模型端点提供者（由 engine 装配包调用）。
//
// 不主动清空 modelOnce：配置读取时机仍保持“首次模型调用时解析并缓存”，
// 测试切换端点后需显式调用 ResetModelCache（与迁移前一致）。
func SetModelEndpointProvider(provider ModelEndpointProvider) {
	endpointMu.Lock()
	endpointProvider = provider
	endpointMu.Unlock()
}

// currentModelEndpoint 读取当前端点提供者。
func currentModelEndpoint() (ModelEndpoint, bool) {
	endpointMu.RLock()
	provider := endpointProvider
	endpointMu.RUnlock()
	if provider == nil {
		return ModelEndpoint{}, false
	}
	return provider(), true
}
