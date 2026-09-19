package handlers

import (
	"LunarAstral/engine"
	"LunarAstral/websocket"
	"encoding/json"
	"io"
	"net/http"
)

func MessageBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "MessageBatch请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req MessageBatchRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "MessageBatch请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	engine.EnqueueMessage(req.Messages...)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  engine.UnreadCount(),
	})
}

// EngineMessageHandler 接收引擎/工作室系统消息（POST /write/engine）
// 与对话消息端点 /write/message 分属两个系统：此处仅更新智能体查询所需的缓存
// （动画列表/位置遥测），引擎事件不进入对话上下文
func EngineMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "EngineMessage请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "EngineMessage请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var msg EngineMessage
	if err = json.Unmarshal(body, &msg); err != nil {
		http.Error(w, "EngineMessage请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}
	if msg.Type == "" {
		http.Error(w, "EngineMessage请求[ERROR] -> 缺少 type 字段", http.StatusBadRequest)
		return
	}

	switch msg.Type {
	case "animation_list":
		// 缓存可用动作定义，供智能体 getAvailableActions 查询
		websocket.CacheAnimationList(body)
	case "telemetry":
		// 提取角色 3D 位置，更新缓存供智能体 getAgentPosition 查询
		var tp struct {
			Character *struct {
				X float64 `json:"x"`
				Y float64 `json:"y"`
				Z float64 `json:"z"`
			} `json:"character"`
		}
		if err := json.Unmarshal(msg.Payload, &tp); err == nil && tp.Character != nil {
			engine.UpdateAgentPosition(tp.Character.X, tp.Character.Y, tp.Character.Z)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}
