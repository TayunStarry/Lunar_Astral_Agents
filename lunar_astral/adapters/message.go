package adapters

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/json"

	"github.com/dop251/goja"
)

func (class *Runtime) pushContext(msgType string, content string, audio string) goja.Value {
	data := PushContextData{
		Type:    msgType,
		Content: content,
		Audio:   audio,
	}
	PushMessageFunc("context", data)

	return class.runtime.ToValue(true)
}

func (class *Runtime) pushImage(images []string) goja.Value {
	data := PushImageData{
		Type:   "image",
		Images: images,
	}
	PushMessageFunc("image", data)
	return class.runtime.ToValue(true)
}

// pullContext 拉取上下文消息
func (class *Runtime) pullContext() goja.Value {
	// 如果未处理的上下文消息为空，返回空数组
	if len(UnreadContext) == 0 {
		return class.runtime.ToValue([]PostMessage{})
	}
	ctxJson, _ := json.Marshal(UnreadContext)
	// 清空未处理的上下文消息
	UnreadContext = make([]PostMessage, 0)
	// 定义响应格式
	var response []any
	// 将JSON字符串解析为响应格式
	json.Unmarshal(ctxJson, &response)
	// 返回拉取到的上下文消息
	return class.runtime.ToValue(response)
}

// pullVideoUrl 拉取视频URL
func (class *Runtime) pullVideoUrl() goja.Value {
	// 如果未处理的视频URL为空，返回空数组
	if len(UnreadVideoUrl) == 0 {
		return class.runtime.ToValue([]string{})
	}
	// 拷贝未处理的视频URL
	url := append([]string{}, UnreadVideoUrl...)
	// 清空未处理的视频URL
	UnreadVideoUrl = make([]string, 0)
	// 返回拉取到的视频URL
	return class.runtime.ToValue(url)
}

// getAgentPosition 获取缓存的智能体3D位置（由前端遥测数据更新）
func (class *Runtime) getAgentPosition() goja.Value {
	agentPositionMutex.RLock()
	defer agentPositionMutex.RUnlock()
	return class.runtime.ToValue(map[string]interface{}{
		"x": agentPosition.X,
		"y": agentPosition.Y,
		"z": agentPosition.Z,
	})
}

// pushAgentEvent 将 3D 引擎事件推送到 AI 上下文
// eventType: 事件类型（如 "movement_complete", "action_started"）
// data: 事件数据的 JSON 字符串
func (class *Runtime) pushAgentEvent(eventType string, data string) goja.Value {
	message := PostMessage{
		Role: "system",
		Content: map[string]string{
			"type":  "agent_event",
			"event": eventType,
			"data":  data,
		},
	}
	UnreadContext = append(UnreadContext, message)
	LoggerGeneral.Info("LunarCore", "[智能体事件] %s: %s", eventType, data)
	return class.runtime.ToValue(true)
}

// UpdateAgentPosition 供 HTTP handler 调用，更新缓存的智能体位置
// 该函数不在 JS 运行时中导出，仅由 Go 端调用
func UpdateAgentPosition(x, y, z float64) {
	agentPositionMutex.Lock()
	defer agentPositionMutex.Unlock()
	agentPosition.X = x
	agentPosition.Y = y
	agentPosition.Z = z
}

// PushAgentEventToContext 供 HTTP handler 调用，将引擎事件推送到 AI 上下文
// 该函数不在 JS 运行时中导出，仅由 Go 端调用
func PushAgentEventToContext(eventType string, data string) {
	message := PostMessage{
		Role: "system",
		Content: map[string]string{
			"type":  "agent_event",
			"event": eventType,
			"data":  data,
		},
	}
	UnreadContext = append(UnreadContext, message)
	LoggerGeneral.Info("LunarCore", "[智能体事件] %s: %s", eventType, data)
}
