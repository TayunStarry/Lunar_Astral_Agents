package adapters

import (
	"encoding/json"
	"logger"
	"time"

	"github.com/dop251/goja"
)

// ==== 引擎桥接器：Agent → 本地 StudioHub → 引擎（自包含，不依赖 crystal_astral） ====

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
// 消息直接广播到本地 StudioHub，无需经过 crystal_astral
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

	cmdJSON, err := json.Marshal(cmd)
	if err != nil {
		logger.Error("LunarCore", "[引擎桥接] 命令序列化失败: %v", err)
		return class.runtime.ToValue(false)
	}

	StudioBroadcastFunc(cmdJSON)
	logger.Info("LunarCore", "[引擎桥接] 命令已广播: type=%s", msgType)
	return class.runtime.ToValue(true)
}

// getAvailableActions 查询引擎当前可用的动作列表（供 JS 运行时调用）
// 返回值：JSON 字符串，格式为 {"actions":[...], "updated_at":...}
// 数据来源于本地 StudioHub 从引擎 animation_list 消息中提取的缓存
func (class *Runtime) getAvailableActions(call goja.FunctionCall) goja.Value {
	cache := GetAnimCacheFunc()
	if cache == nil {
		return class.runtime.ToValue("{}")
	}

	body, err := json.Marshal(cache)
	if err != nil {
		logger.Warn("LunarCore", "[引擎桥接] 动画缓存序列化失败: %v", err)
		return class.runtime.ToValue("{}")
	}

	return class.runtime.ToValue(string(body))
}