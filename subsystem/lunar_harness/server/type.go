package server

import (
	"net/http"

	"LunarSubsystem/LunarHarness/protocol/endpoint"
)

// StaticRoots 两个桌面前端的静态资源根
type StaticRoots struct {
	// Lunar 月华消息终端（挂载于 /）
	Lunar http.FileSystem
	// Crystal 琉璃桌面（挂载于 /desktop/）
	Crystal http.FileSystem
}

// Ready 两个前端的静态资源是否均已就绪
func (r StaticRoots) Ready() bool {
	return r.Lunar != nil && r.Crystal != nil
}

// EngineResponse /write/engine 响应
type EngineResponse struct {
	// Success 是否受理成功
	Success bool `json:"success"`
	// Length 透传的原始字节长度
	Length int `json:"length"`
	// Observed 被反射观测到的消息类型列表
	Observed []string `json:"observed"`
	// Broadcast 本次广播触达的在线客户端数
	Broadcast int `json:"broadcast"`
}

// WindowFocusRequest /window/focus 请求体
type WindowFocusRequest struct {
	// ID 目标桌面窗口标识（lunar / crystal）
	ID string `json:"id"`
}

// WindowFocusResponse /window/focus 响应体
type WindowFocusResponse struct {
	// Success 是否成功
	Success bool `json:"success"`
	// Message 结果说明
	Message string `json:"message"`
}

// SystemEndpoint 统一路由注册表元素（别名，便于各侧复用同一类型）
type SystemEndpoint = endpoint.SystemEndpoint
