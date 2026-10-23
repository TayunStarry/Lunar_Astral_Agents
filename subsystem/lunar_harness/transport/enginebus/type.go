package enginebus

import (
	"encoding/json"
	"reflect"
	"sync"
)

// Envelope /write/engine 的通用信封（仅用于识别 type 与 payload，不改变透传字节）
type Envelope struct {
	// Type 消息类型（如 animation_list / telemetry）
	Type string `json:"type"`
	// Payload 消息负载（原样保留）
	Payload json.RawMessage `json:"payload"`
}

// Observer 引擎总线观察者。
// 约定：实现 On<Type> 形式的导出方法即自动被反射索引，方法名按下划线规则映射到消息 type：
//
//	OnAnimationList(raw []byte) → type "animation_list"
//	OnTelemetry(raw []byte)     → type "telemetry"
//
// 新增观察者只需新增方法，无需改动任何注册表。
type Observer interface {
	// Name 观察者名称（日志与自检用）
	Name() string
}

// Sender 广播发送函数（由装配层注入，避免 transport/enginebus 反向依赖 transport/ws）
type Sender func([]byte)

// Bus 引擎消息总线：反射观测 + 原样透传。
type Bus struct {
	mu sync.RWMutex
	// methods 消息 type → 已绑定的观察者方法
	methods map[string]reflect.Value
	// owners 消息 type → 观察者名称
	owners map[string]string
	// sender 透传发送函数
	sender Sender
}
