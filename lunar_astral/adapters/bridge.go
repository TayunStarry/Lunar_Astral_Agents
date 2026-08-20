package adapters

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"
	"time"

	"github.com/dop251/goja"
)

// ==== 引擎桥接器：Agent → 标准 ws 广播 → 引擎 ====
// 引擎命令经 PushMessageFunc 以 {type:"engine", data:EngineCommand} 广播给所有 /ws 客户端
// 引擎所在域的枢纽（如 crystal_astral 上游桥）接收后转发给本地引擎客户端

// 引擎命令消息格式（与 engine.js 的 handleChannelMessage 对齐）
type EngineCommand struct {
	Type      string      `json:"type"`
	Source    string      `json:"source"`
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

const engineCommandSource = "lunar-agent"

// sendToEngine 向引擎发送命令（供 JS 运行时调用）
// 用法：sendToEngine('action', '{"action": "荡秋千"}')
// 消息以标准 ws 广播推送，经上游桥转发后由引擎按 data.type 过滤处理
func (class *Runtime) sendToEngine(call goja.FunctionCall) goja.Value {
	msgType := call.Argument(0).String()
	payloadStr := call.Argument(1).String()

	var payload interface{}
	if payloadStr != "" && payloadStr != "undefined" {
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			payload = payloadStr
		}
	} else {
		payload = nil
	}

	cmd := EngineCommand{
		Type:      msgType,
		Source:    engineCommandSource,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}

	PushMessageFunc("engine", cmd)
	LoggerGeneral.Info("LunarCore", "[引擎桥接] 命令已广播: type=%s", msgType)
	return class.runtime.ToValue(true)
}

// getAvailableActions 查询引擎当前可用的动作列表（供 JS 运行时调用）
// 返回值：JSON 字符串，格式为 {"actions":[...], "updated_at":...}
// 数据来源于引擎 animation_list 消息经 /write/engine 更新后的缓存
func (class *Runtime) getAvailableActions(call goja.FunctionCall) goja.Value {
	cache := GetAnimCacheFunc()
	if cache == nil {
		return class.runtime.ToValue("{}")
	}

	body, err := json.Marshal(cache)
	if err != nil {
		LoggerGeneral.Warn("LunarCore", "[引擎桥接] 动画缓存序列化失败: %v", err)
		return class.runtime.ToValue("{}")
	}

	return class.runtime.ToValue(string(body))
}
