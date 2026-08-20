package handlers

import (
	"LunarAstral/adapters"
	"LunarAstral/websocket"
	"LunarSubsystem/LoggerGeneral"
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

	for _, msg := range req.Messages {
		adapters.UnreadContext = append(adapters.UnreadContext, msg)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  len(adapters.UnreadContext),
	})
}

func VideoUrlBatchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req VideoUrlBatchRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "VideoUrlBatch请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	for _, url := range req.Urls {
		adapters.UnreadVideoUrl = append(adapters.UnreadVideoUrl, url)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  len(adapters.UnreadVideoUrl),
	})
}

// AgentPositionHandler 接收前端遥测数据，更新缓存的智能体3D位置
func AgentPositionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "AgentPosition请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "AgentPosition请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req AgentPositionRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "AgentPosition请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	adapters.UpdateAgentPosition(req.X, req.Y, req.Z)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}

// AgentEventHandler 接收前端引擎事件，推送到 AI 上下文
func AgentEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "AgentEvent请求[ERROR] -> 不允许的请求方法", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "AgentEvent请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req AgentEventRequest
	if err = json.Unmarshal(body, &req); err != nil {
		http.Error(w, "AgentEvent请求[ERROR] -> 解析请求体失败", http.StatusBadRequest)
		return
	}

	adapters.PushAgentEventToContext(req.Event, req.Data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}

// EngineMessageHandler 接收引擎/工作室系统消息（POST /write/engine）
// 与对话消息端点 /write/message 分离：按消息 type 分发处理，
// 引擎消息不进入对话上下文，仅更新智能体查询所需的缓存（动画列表/位置）
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
			adapters.UpdateAgentPosition(tp.Character.X, tp.Character.Y, tp.Character.Z)
		}
	default:
		// 其他引擎消息（模块间互发等）由 crystal_astral 转发广播，智能体侧忽略
		LoggerGeneral.Info("LunarCore", "[引擎消息] 已忽略: type=%s source=%s", msg.Type, msg.Source)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(BatchResponse{
		Success: true,
		Length:  1,
	})
}
