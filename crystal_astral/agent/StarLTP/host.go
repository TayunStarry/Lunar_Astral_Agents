// Package StarLTP 实现星月智能 StarLTP 引擎：每插件独立 goja 沙箱 + engine.* 白名单 API + 代码哈希权限密钥。
// 本包对外暴露 Go 层引擎接口，供任意 Go 程序不经 WebSocket 链路直接调用引擎功能。
package StarLTP

// ==== 公开 Go 引擎接口（不经 WS 直接调用） ====

import (
	"fmt"

	"LunarSubsystem/LoggerGeneral"
)

// Init 初始化 LTP9 引擎：扫描 LTP9 包根目录并加载全部插件（幂等）。
func Init() error {
	if Engine != nil {
		return nil
	}
	Engine = newEngine()
	Engine.LoadAll()
	LoggerGeneral.Info(ServiceName, "LTP9 引擎已初始化，插件数: %d", len(Engine.plugins))
	return nil
}

// Version 返回引擎构建版本标记（供运行实例自证是否最新）。
func Version() string { return EngineBuild }

// Close 关闭引擎并卸载全部插件。
func Close() {
	if Engine != nil {
		Engine.shutdown()
	}
}

// SetOutbound 注入插件单向通报接收函数（engine.signal 的目标，供外部 Go 程序消费）。
func SetOutbound(fn func(topic string, payload any)) {
	if Engine != nil {
		Engine.setOutbound(fn)
	}
}

// SetAgentInvoker 注入前端智能体调用实现（engine.agent 的真实通道）。
// 由宿主（crystal_astral）提供：把 appID + 自然语言转交 Mini-LTP / Node-LTP 包并返回其执行文本。
// 未注入时，engine.agent 返回「xxx 包拒绝响应」。
func SetAgentInvoker(fn func(appID, instruction string) (string, error)) {
	outboundMu.Lock()
	agentInvoker = fn
	outboundMu.Unlock()
}

// SetSendInvoker 注入发送通道（engine.send 的真实实现）。
// kind 取值 image（target=群ID，payload=base64 字符串）/ text / hybrid（payload=分段数组）。
// 未注入时，engine.send 返回错误提示。
func SetSendInvoker(fn func(kind, target string, payload any) error) {
	sendMu.Lock()
	sendInvoker = fn
	sendMu.Unlock()
}

// SetWsServer 注入 WebSocket 服务端传输（engine.ws 的真实实现，供插件挂载流地址并广播）。
// 未注入时，engine.ws 返回错误提示。
func SetWsServer(bridge *WsBridge) {
	wsMu.Lock()
	wsServicer = bridge
	wsMu.Unlock()
}

// PluginStates 返回全部插件状态（供外部 Go 程序枚举）。
func PluginStates() []PluginState {
	if Engine == nil {
		return nil
	}
	return Engine.pluginStates()
}

// Rescan 重新扫描并加载/卸载插件（热更新）。
func Rescan() {
	if Engine != nil {
		Engine.reconcile()
	}
}

// Emit 由客户端（外部 Go 程序）直接发起一个事件，路由到所有订阅该 topic 的插件订阅器。
// 返回各插件处理结果与汇总（含拦截/撤回状态）。
func Emit(topic string, payload any, requestID string) EmitResult {
	result := EmitResult{Topic: topic}
	if Engine == nil {
		return result
	}
	summary := EmitSummary{}
	cur := payload
	for _, p := range Engine.snapshotPlugins() {
		p.mu.Lock()
		hasSubs := len(p.events[topic]) > 0
		p.mu.Unlock()
		if !hasSubs {
			continue
		}
		outs := p.fireEvent(topic, cur)
		if len(outs) == 0 {
			summary.Subscribed++
			continue
		}
		for _, oc := range outs {
			summary.Subscribed++
			if oc.Error != "" {
				summary.Errored++
				continue
			}
			if m, ok := oc.Result.(map[string]any); ok {
				// 改写：事件负载沿订阅器链向下游（含其余插件）传递
				if v, has := m["modifiedData"]; has && v != nil {
					cur = v
				}
				if m["intercept"] == true {
					summary.Intercepted = true
				}
				if m["cancel"] == true {
					summary.Canceled = true
				}
			}
		}
		result.Outcomes = append(result.Outcomes, outs...)
	}
	result.Summary = summary
	return result
}

// Call 由外部 Go 程序直接调用某个插件导出的函数（等价于 engine.call）。
// 目标插件不存在或未导出该函数 → 返回 xxx 包拒绝响应。
func Call(pluginID, fnName string, args []any) (any, error) {
	if Engine == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", pluginID)
	}
	return engineCall(pluginID, fnName, args)
}

// Agent 由外部 Go 程序直接调用前端智能体（Mini-LTP / Node-LTP）。
// 目标包不存在或智能体不存在 → 返回 xxx 包拒绝响应。
func Agent(pluginID, naturalLang string) (any, error) {
	return engineAgent(pluginID, naturalLang)
}

// Broadcast 由外部 Go 程序向所有插件的 frontEvent.signal 广播一条内容。
func Broadcast(payload any) {
	engineBroadcast("__all__", payload)
}

// BroadcastTo 由外部 Go 程序向指定插件的 frontEvent.signal 广播一条内容。
func BroadcastTo(pluginID string, payload any) {
	engineBroadcast(pluginID, payload)
}