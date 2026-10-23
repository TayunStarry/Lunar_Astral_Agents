package lunarloop

import (
	"encoding/json"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// ==== 引擎消息观察者（/write/engine 反射观测端） ====
//
// /write/engine 的「反射 + 透传」设计：web 客户端发来的消息在服务端原样反弹广播，
// 同时由引擎消息总线用反射按方法名索引分派给以下观察者做**只读观测**，
// 把引擎侧状态缓存下来，供智能体查询（getAvailableActions / getAgentPosition）。
//
// 约定：方法名 OnXxx 自动映射到消息 type 的 snake_case（OnAnimationList → animation_list、
// OnTelemetry → telemetry），新增观测能力只需新增方法，无需改动任何注册表。

// AnimationObserver 观测动画列表消息，更新可用动作缓存
type AnimationObserver struct{}

// Name 观察者名称
func (AnimationObserver) Name() string { return "AnimationListCache" }

// OnAnimationList 对应消息 type "animation_list"：缓存动作定义供 getAvailableActions 查询
func (AnimationObserver) OnAnimationList(raw []byte) {
	CacheAnimationList(raw)
}

// TelemetryObserver 观测位置遥测消息，更新角色 3D 位置缓存
type TelemetryObserver struct{}

// Name 观察者名称
func (TelemetryObserver) Name() string { return "AgentPosition" }

// OnTelemetry 对应消息 type "telemetry"：提取角色位置供 getAgentPosition 查询
func (TelemetryObserver) OnTelemetry(raw []byte) {
	var env struct {
		Payload struct {
			Character *struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"character"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		logger.SubWarn("LunarCore", "EngineBus", "位置遥测解析失败: %v", err)
		return
	}
	if env.Payload.Character == nil {
		return
	}
	UpdateAgentPosition(env.Payload.Character.X, env.Payload.Character.Y, env.Payload.Character.Z)
}
