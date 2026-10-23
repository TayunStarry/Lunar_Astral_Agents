package server

import (
	"encoding/json"
	"io"
	"net/http"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/transport/enginebus"
	"LunarSubsystem/LunarHarness/transport/ws"
)

// EngineMessageHandler 引擎/前端系统总线（POST /write/engine）。
//
// 反射 + 透传设计：
//  1. 原始字节读入后**永不改写**；
//  2. 反射阶段：解析信封 type，反射分派给已注册观察者做只读观测（动画列表缓存、位置遥测等），
//     观测异常仅记录日志，绝不阻断透传；未识别 type 与缺失 type 均跳过观测；
//  3. 透传阶段：原始字节经统一集线器无差别广播给全部 /ws 客户端（客户端自行过滤）；
//  4. 服务端保留主动发送能力：enginebus.Bus.Emit 可构造并广播引擎消息。
//
// 引擎事件不进入对话上下文（与 /write/message 分属两个系统）。
func EngineMessageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "引擎消息请求[ERROR] -> 读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// ---- 反射阶段：只读观测（失败不影响透传） ----
	observed := []string{}
	if enginebus.Instance != nil {
		observed = enginebus.Instance.Publish(body)
	}

	// ---- 透传阶段：原样投递给引擎总线订阅者（消息终端等非订阅者不收） ----
	broadcast := 0
	if ws.Instance != nil {
		// 频道由发送方声明（?channel=…，缺省默认频道）：总线帧只投递给同频道的订阅者，
		// 从而隔离同一进程内彼此独立的引擎实例（月华内嵌渲染引擎 / 琉璃 studio 编辑器引擎）
		ws.Instance.BroadcastBus(ws.RequestChannel(r), body)
		broadcast = ws.Instance.BusCount()
	} else {
		logger.Warn("LunarHarness", "引擎消息到达但集线器未就绪，仅完成观测")
	}

	writeJSON(w, http.StatusOK, EngineResponse{
		Success:   true,
		Length:    len(body),
		Observed:  observed,
		Broadcast: broadcast,
	})
}

// EngineEmitHandler 由服务端主动构造并广播一条引擎消息（保留的发送能力入口）。
// 路由默认不注册，仅在需要外部触发时启用；内部调用方直接使用 enginebus.Emit。
func EngineEmitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var env enginebus.Envelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil || env.Type == "" {
		http.Error(w, "引擎消息请求[ERROR] -> 请求体需包含 type 字段", http.StatusBadRequest)
		return
	}
	if enginebus.Instance == nil {
		http.Error(w, "引擎消息请求[ERROR] -> 引擎总线未就绪", http.StatusServiceUnavailable)
		return
	}
	if err := enginebus.Instance.Emit(env.Type, json.RawMessage(env.Payload)); err != nil {
		http.Error(w, "引擎消息请求[ERROR] -> "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, EngineResponse{Success: true, Length: len(env.Payload)})
}
