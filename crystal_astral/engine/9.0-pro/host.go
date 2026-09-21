// Package ltp9 实现星月智能 LTP9 引擎（琉璃基板桥接版）：每插件独立 goja 沙箱 + engine.* 白名单 API +
// 代码哈希权限密钥 + 模型垄断（engine.llm 强制 agent 字段模型）。
// 本包对外暴露 Go 层引擎接口，供任意 Go 程序不经 WebSocket 链路直接调用引擎功能。
package ltp9

// ==== 公开 Go 引擎接口（不经 WS 直接调用） ====

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"LunarSubsystem/LoggerGeneral"
)

// Init 初始化 LTP9 引擎：扫描 LTP9 包根目录并加载全部插件（幂等）。
func Init() error {
	if Engine != nil {
		return nil
	}
	Engine = newEngine()
	Engine.LoadAll()
	LoggerGeneral.Info(ServiceName, "LTP9 引擎已初始化 plugins=%d", len(Engine.plugins))
	return nil
}

// Version 返回引擎构建版本标记（供运行实例自证是否最新）。
func Version() string { return EngineBuild }

// AgentPackages 扫描包根目录，返回带 Mini-LTP / Node-LTP 标签的包 ID（engine.agent 的可选目标）。
func AgentPackages() []string {
	root := packageRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(root, ent.Name(), "metadata.json"))
		if rerr != nil {
			continue
		}
		var m pkgMeta
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		id := m.ID
		if id == "" {
			id = ent.Name()
		}
		for _, t := range m.Tags {
			if strings.EqualFold(t, "Mini-LTP") || strings.EqualFold(t, "Node-LTP") {
				out = append(out, id)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// Close 关闭引擎并卸载全部插件。
func Close() {
	if Engine != nil {
		Engine.shutdown()
	}
	saveModelStats()
}

// SetModelInvoker 注入 engine.llm.chat 的下层传输实现（模型垄断语义）。
// 无论插件传什么参数，注入的 invoker 收到的 opts 都会被强制覆盖为 agent 对话模型
// （model/baseUrl/apiKey 全部改写为 GeneralConfig.AgentMultimodal* 字段），保证模型垄断。
func SetModelInvoker(fn func(messages []any, opts map[string]any) (any, error)) {
	modelInvokerMu.Lock()
	modelInvoker = fn
	modelInvokerMu.Unlock()
}

// SetEmbedInvoker 注入 engine.llm.embed 的下层传输实现（模型垄断语义）。
// 注入的 invoker 收到的 opts 会被强制覆盖为 agent 嵌入模型（GeneralConfig.AgentEmbedding*）。
func SetEmbedInvoker(fn func(input any, opts map[string]any) (any, error)) {
	embedInvokerMu.Lock()
	embedInvoker = fn
	embedInvokerMu.Unlock()
}

// SetOutbound 注入插件单向通报接收函数（engine.signal 的目标，供外部 Go 程序消费）。
func SetOutbound(fn func(topic string, payload any)) {
	if Engine != nil {
		Engine.setOutbound(fn)
	}
}

// SetAgentInvoker 注入前端智能体调用实现（engine.agent 的真实通道）。
func SetAgentInvoker(fn func(appID, instruction string) (string, error)) {
	outboundMu.Lock()
	agentInvoker = fn
	outboundMu.Unlock()
}

// SetSendInvoker 注入发送通道（engine.send 的真实实现）。
func SetSendInvoker(fn func(pluginID, kind, target string, payload any) error) {
	sendMu.Lock()
	sendInvoker = fn
	sendMu.Unlock()
}

// SetWsServer 注入 WebSocket 服务端传输（engine.ws 的真实实现）。
func SetWsServer(bridge *WsBridge) {
	wsMu.Lock()
	wsServicer = bridge
	wsMu.Unlock()
}

// SetPlatformResolver 注入平台能力解析器（engine.platform 的真实实现）。
func SetPlatformResolver(fn func(method string, args map[string]any) (any, error)) {
	platformMu.Lock()
	platformResolver = fn
	platformMu.Unlock()
}

// SetDeveloperMode 设置开发模式。默认 false 启用 permissions.key 代码哈希校验；
// 传 true 跳过校验并授予全部权限。
func SetDeveloperMode(on bool) {
	DeveloperMode = on
}

// SetLocalDir 设置 LTP9 运行时数据根目录（应在 Init 前调用）。
// 桥接琉璃基板：封包根目录默认落在 GeneralConfig.LocalDir/package；宿主显式指定时优先。
func SetLocalDir(dir string) {
	if dir != "" {
		LocalDir = dir
	}
}

// SetRootOverride 显式指定 LTP9 包根目录（应在 Init 前调用）。
func SetRootOverride(root string) {
	if root != "" {
		rootOverride = root
	}
}

// CallTool 由外部 Go 程序直接调用某个插件注册的工具。
func CallTool(pluginID, name string, args map[string]any) (any, error) {
	return engineToolCall(pluginID, name, args, nil)
}

// CallToolWithContext 同 CallTool，额外透传调用上下文。
func CallToolWithContext(pluginID, name string, args map[string]any, context map[string]any) (any, error) {
	return engineToolCall(pluginID, name, args, context)
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
func Emit(topic string, payload any, requestID string) EmitResult {
	result := EmitResult{Topic: topic}
	if Engine == nil {
		return result
	}
	summary := EmitSummary{}
	cur := payload
	modified := false
	var returned any
	hasReturn := false
	for _, p := range Engine.snapshotPlugins() {
		p.mu.Lock()
		hasSubs := len(p.events[topic]) > 0
		loaded := p.loaded
		p.mu.Unlock()
		if !loaded {
			continue
		}
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
				if v, has := m["modifiedData"]; has && v != nil {
					cur = v
					modified = true
				}
				if v, has := m["return"]; has && v != nil {
					returned = v
					hasReturn = true
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
		if summary.Canceled {
			break
		}
	}
	result.Summary = summary
	result.Data = cur
	result.Modified = modified
	result.Returned = hasReturn
	result.Return = returned
	return result
}

// Call 由外部 Go 程序直接调用某个插件导出的函数（等价于 engine.call）。
func Call(pluginID, fnName string, args []any) (any, error) {
	if Engine == nil {
		return nil, fmt.Errorf("%s 包拒绝响应", pluginID)
	}
	return engineCall(pluginID, fnName, args)
}

// Agent 由外部 Go 程序直接调用前端智能体（Mini-LTP / Node-LTP）。
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

// SetCommandSync 由宿主注入命令登记回调（插件注册 engine.command 时回调）。
func SetCommandSync(fn func(pluginID, name, pattern string)) {
	commandSyncFn = fn
}

// CurrentLocalDir 返回当前生效的运行时数据根目录（供宿主自查，桥接基板一致性）。
func CurrentLocalDir() string {
	return localBase()
}

// CurrentRoot 返回当前生效的 LTP9 包根目录。
func CurrentRoot() string {
	return packageRoot()
}