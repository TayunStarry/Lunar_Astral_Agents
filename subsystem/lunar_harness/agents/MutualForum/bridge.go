package MutualForum

// bridge.go 桥接层与引擎层的最小衔接：构造客户端与幂等键。
//
// HTTP 传输、信封解析、错误映射与 API DTO 全部位于 engine/forum；
// 本文件不再持有任何 request 实现，避免引擎能力在智能体层重复。

import (
	engineforum "LunarSubsystem/LunarHarness/engine/forum"
)

// newClient 按配置构造客户端（不带凭证；凭证由 EnsureAccount 填充）。
func newClient(cfg Config) *Client {
	return engineforum.New(cfg.Endpoint(), cfg.RequestTimeout())
}

// newRequestID 生成可读的幂等键（client_request_id）。
func newRequestID(prefix string) string {
	return engineforum.NewRequestID(prefix)
}
