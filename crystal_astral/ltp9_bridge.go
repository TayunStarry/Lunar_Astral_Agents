package main

// ==== LTP9 引擎接线（主机注入真实通道） ====
// 由 crystal_astral 启动时调用，把项目本地的记忆库/数据库/前端智能体真实能力连接到 LTP9 引擎。
// 说明：LTP9 引擎本身已直接接入项目记忆库（FileManager/module）与 SQLite；
// 前端智能体（Mini-LTP / Node-LTP）通道依赖宿主 WS 前端包调用，故在此注入 ltpInvokeFrontAgent。

import (
	"encoding/json"
	"fmt"
	"time"

	star "CrystalAstral/agent/StarLTP"
	"LunarSubsystem/LoggerGeneral"
)

// BridgeLTP9 初始化 LTP9 引擎并注入主机真实通道（幂等）。
func BridgeLTP9() error {
	var rerr error
	if err := star.Init(); err != nil {
		rerr = err
	}
	// 前端智能体真实通道：appID + 自然语言 → 前端 Mini-LTP/Node-LTP 包执行并返回文本
	star.SetAgentInvoker(ltpInvokeFrontAgent)
	// 插件单向通报 → 引擎日志（可在此转发到 /ws 等）
	star.SetOutbound(func(topic string, payload any) {
		LoggerGeneral.Info("StarLTP", "插件单向通报 %s: %v", topic, payload)
	})
	// engine.send：经主 /ws 集线器广播（群聊直发不在本进程，由前端/代理消费）
	star.SetSendInvoker(ltpPushSend)
	// engine.ws：为插件提供独立回环 WebSocket 流端点
	star.SetWsServer(&star.WsBridge{
		Serve:   ltp9WSServe,
		Publish: ltp9WSPublish,
	})
	return rerr
}

// ltpPushSend engine.send 的宿主通道：把 image/text/hybrid 请求封装为枚举转发到 /ws 集线器。
// 说明：crystal_astral 不直接对接群聊发送（由 lunar_astral/napcat 或前端代理负责），故此处做桥接广播。
func ltpPushSend(kind, target string, payload any) error {
	if StudioHubInstance == nil {
		return fmt.Errorf("发送通道未就绪（前端总线未启动）")
	}
	msg, _ := json.Marshal(map[string]any{
		"type":    "ltp9_send",
		"kind":    kind,
		"target":  target,
		"payload": payload,
		"time":    time.Now().Unix(),
	})
	select {
	case StudioHubInstance.Broadcast <- msg:
		return nil
	default:
		return fmt.Errorf("发送广播缓冲区已满")
	}
}

// ltpInvokeFrontAgent 复用 LTPX 前端包调用链路：登记待定调用 → /ws 广播 ltpx_call → 等待包回执。
// 等价于 ltpRemoteCallHandler 的「前端包工具」分支，供 LTP9 engine.agent 调用 Mini-LTP / Node-LTP。
func ltpInvokeFrontAgent(appID, instruction string) (string, error) {
	if StudioHubInstance == nil {
		return "", fmt.Errorf("%s 包拒绝响应（前端总线未就绪）", appID)
	}
	if appID == "" {
		return "", fmt.Errorf("包 ID 为空")
	}
	requestID := fmt.Sprintf("ltp9-ag-%d", time.Now().UnixNano())
	done := make(chan LTPXRemoteCallResponse, 1)
	ltpPendingMutex.Lock()
	ltpPendingCalls[requestID] = done
	ltpPendingMutex.Unlock()

	msg, _ := json.Marshal(map[string]any{
		"type":       "ltpx_call",
		"request_id": requestID,
		"app_id":     appID,
		"tool":       "",
		"arguments":  map[string]any{"instruction": instruction},
	})
	StudioHubInstance.Broadcast <- msg

	select {
	case resp := <-done:
		if !resp.Success {
			return resp.Text, fmt.Errorf("%s 包执行失败: %s", appID, firstNonEmpty(resp.Error, "未知错误"))
		}
		return resp.Text, nil
	case <-time.After(ltpCallTimeout):
		ltpPendingMutex.Lock()
		delete(ltpPendingCalls, requestID)
		ltpPendingMutex.Unlock()
		return "", fmt.Errorf("%s 包拒绝响应（前端调用超时 %s）", appID, ltpCallTimeout)
	}
}

// firstNonEmpty 返回首个非空字符串。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}