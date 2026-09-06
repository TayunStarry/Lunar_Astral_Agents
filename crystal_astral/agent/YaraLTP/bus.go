package YaraLTP

// ==== WS 总线：引擎层与真实客户端之间的 ltp3.* 信封收发 ====

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"

	"LunarSubsystem/LoggerGeneral"
)

// ==== 入站分发 worker 池 ====
// 把 ltp3/* 信封从串行消费者解耦为后台 worker 并发分发：单个慢工具/慢钩子不再
// 阻塞 ping、其它插件调用与其它群聊。同插件调用仍由该插件自身的互斥锁串行化，
// 保证 goja 虚拟机不被并发访问（不同插件之间则真正并行）。

// inboundMu 保护 inboundJobs / inboundStop 的启停操作。
var inboundMu sync.Mutex

// startInboundWorkers 启动入站分发 worker 池（Init 时调用，幂等）。
func startInboundWorkers() {
	inboundMu.Lock()
	defer inboundMu.Unlock()
	if inboundJobs != nil {
		return
	}
	jobs := make(chan *InMessage, inboundQueueCap)
	stop := make(chan struct{})
	inboundJobs = jobs
	inboundStop = stop
	for i := 0; i < inboundWorkerCount; i++ {
		go func() {
			for {
				select {
				case msg := <-jobs:
					if msg == nil {
						return
					}
					processIn(msg)
				case <-stop:
					return
				}
			}
		}()
	}
}

// stopInboundWorkers 停止 worker 池（Close 时调用）。仅关闭停止信号，
// 不等待正在执行的慢调用，避免关停被长时间挂起。
func stopInboundWorkers() {
	inboundMu.Lock()
	defer inboundMu.Unlock()
	if inboundStop == nil {
		return
	}
	close(inboundStop)
	inboundJobs = nil
	inboundStop = nil
}

// dispatchInbound 把入站信封交给 worker 池异步处理；队列满时回退为直接后台处理，
// 保证调用方（串行消费者）永不阻塞。
func dispatchInbound(msg *InMessage) {
	inboundMu.Lock()
	jobs := inboundJobs
	inboundMu.Unlock()
	if jobs == nil {
		go processIn(msg)
		return
	}
	select {
	case jobs <- msg:
	default:
		go processIn(msg)
	}
}

// emitBus 把结构化消息 JSON 化后由发送函数广播到 /ws（client 端按 request_id 过滤做单播配对）。
func emitBus(v any) {
	sendMu.RLock()
	fn := sendOut
	sendMu.RUnlock()
	if fn == nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		LoggerGeneral.Warn(ServiceName, "总线消息序列化失败: %v", err)
		return
	}
	fn(data)
}

// SetSend 设置出站广播发送函数（由 crystal_astral 注入）。
func SetSend(fn func([]byte)) {
	sendMu.Lock()
	sendOut = fn
	sendMu.Unlock()
}

// HandleIn 处理从 /ws 集线器收到的请求信封（type 前缀 ltp3/）。
// 解析后投入入站 worker 池异步分发，避免单个慢调用阻塞整个消费者与其它调用。
func HandleIn(raw []byte) {
	var msg InMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return
	}
	t := strings.TrimSpace(msg.Type)
	if !strings.HasPrefix(t, "ltp3/") {
		return // 非本引擎消息，忽略（由其它订阅者处理）
	}
	if Engine == nil {
		return
	}
	dispatchInbound(&msg)
}

// processIn 在 worker 协程中执行一条 ltp3/* 信封的分发，并广播对应回执。
func processIn(msg *InMessage) {
	if Engine == nil || msg == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			LoggerGeneral.Error(ServiceName, "处理 ltp3/%s 崩溃: %v", msg.Type, r)
		}
	}()
	switch strings.TrimSpace(msg.Type) {
	case "ltp3/hook":
		handleHookIn(msg)
	case "ltp3/event":
		handleEventIn(msg)
	case "ltp3/command":
		handleCommandIn(msg)
	case "ltp3/tool":
		handleToolIn(msg)
	case "ltp3/manage":
		handleManageIn(msg)
	case "ltp3/ping":
		emitBus(pongMessage{Type: "ltp3/pong", RequestID: msg.RequestID, Engine: ServiceName, Plugins: Engine.pluginCount()})
	default:
		emitBus(outMessage{Type: "ltp3/error", RequestID: msg.RequestID, Error: "未知类型 " + msg.Type})
	}
}

// handleHookIn 处理钩子发布请求。
func handleHookIn(msg *InMessage) {
	payload := decodePayload(msg.Payload)
	outs, summary := Engine.DispatchHook(msg.Hook, payload, msg.Context, msg.RequestID)
	emitBus(hookResultMessage{
		Type:      "ltp3/hook_result",
		RequestID: msg.RequestID,
		Hook:      msg.Hook,
		Results:   outs,
		Summary:   summary,
	})
}

// handleEventIn 处理事件发布请求。
func handleEventIn(msg *InMessage) {
	payload := decodePayload(msg.Payload)
	cnt := Engine.PublishEvent(msg.Event, payload)
	emitBus(eventAckMessage{Type: "ltp3/event_ack", RequestID: msg.RequestID, Event: msg.Event, Subscribed: cnt})
}

// handleCommandIn 处理指令调用请求。
func handleCommandIn(msg *InMessage) {
	outs, summary := Engine.DispatchCommand(msg.Command, msg.Match, msg.Context, msg.RequestID)
	emitBus(commandResultMessage{
		Type:      "ltp3/command_result",
		RequestID: msg.RequestID,
		Command:   msg.Command,
		Results:   outs,
		Summary:   summary,
	})
}

// handleToolIn 处理工具调用请求。
func handleToolIn(msg *InMessage) {
	params := map[string]any{}
	if p, ok := decodePayload(msg.Payload).(map[string]any); ok {
		params = p
	}
	outs, summary := Engine.DispatchTool(msg.Tool, params, msg.Context, msg.RequestID)
	emitBus(toolResultMessage{
		Type:      "ltp3/tool_result",
		RequestID: msg.RequestID,
		Tool:      msg.Tool,
		Results:   outs,
		Summary:   summary,
	})
}

// handleManageIn 处理管理动作请求（list/scan/reload/reload_one/unload_one）。
func handleManageIn(msg *InMessage) {
	ack := manageAckMessage{Type: "ltp3/manage_ack", RequestID: msg.RequestID, Action: msg.Action, OK: true}
	switch msg.Action {
	case "list":
		ack.Plugins = Engine.states()
	case "scan":
		Engine.reconcile()
		ack.Plugins = Engine.states()
		ack.Message = "对账完成"
	case "reload":
		ids := Engine.ids()
		for _, id := range ids {
			if err := Engine.reloadPackage(id); err != nil {
				ack.OK = false
				ack.Message = err.Error()
				break
			}
		}
		ack.Plugins = Engine.states()
	case "reload_one":
		if err := Engine.reloadPackage(msg.ID); err != nil {
			ack.OK = false
			ack.Message = err.Error()
		}
		ack.Plugins = Engine.states()
	case "unload_one":
		Engine.unloadPackage(msg.ID)
		ack.Plugins = Engine.states()
		ack.Message = "已卸载 " + msg.ID
	default:
		ack.OK = false
		ack.Message = "未知动作 " + msg.Action
	}
	emitBus(ack)
}

// decodePayload 把 RawMessage 解码为 any（保持对象为 map）。
func decodePayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// pluginCount 返回已加载插件数。
func (e *engine) pluginCount() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.plugins)
}

// ids 返回全部插件 ID。
func (e *engine) ids() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]string, 0, len(e.plugins))
	for id := range e.plugins {
		out = append(out, id)
	}
	return out
}

// DispatchTool 分派工具调用：在所有插件中查找同名工具并执行 handler。
func (e *engine) DispatchTool(name string, params map[string]any, ctx map[string]any, requestID string) ([]hookOutcome, dispatchSummary) {
	e.mu.RLock()
	snap := make([]*plugin, 0, len(e.plugins))
	for _, p := range e.plugins {
		if p.tools[name] != nil {
			snap = append(snap, p)
		}
	}
	e.mu.RUnlock()

	summary := dispatchSummary{AllowContinue: true}
	all := []hookOutcome{}
	for _, p := range snap {
		td := p.tools[name]
		p.mu.Lock()
		old := p.currentRequestID
		p.currentRequestID = requestID
		res, err := p.callFn(td.handler, params, ctx)
		p.currentRequestID = old
		p.mu.Unlock()
		oc := hookOutcome{PluginID: p.ID, Handled: true}
		if err != nil {
			oc.Error = err.Error()
			summary.Errored++
		} else {
			oc.Result = res
		}
		summary.Subscribed++
		all = append(all, oc)
	}
	return all, summary
}

// DispatchCommand 分派指令：在所有插件中查找同名指令并执行。
func (e *engine) DispatchCommand(name string, match []string, ctx map[string]any, requestID string) ([]hookOutcome, dispatchSummary) {
	e.mu.RLock()
	snap := make([]*plugin, 0, len(e.plugins))
	for _, p := range e.plugins {
		if p.commands[name] != nil {
			snap = append(snap, p)
		}
	}
	e.mu.RUnlock()
	if len(snap) == 0 {
		// 尝试用正则把整段文本匹配各插件指令
		return e.dispatchCommandByRegex(name, match, ctx, requestID)
	}

	summary := dispatchSummary{AllowContinue: true}
	all := []hookOutcome{}
	for _, p := range snap {
		p.mu.Lock()
		old := p.currentRequestID
		p.currentRequestID = requestID
		res, err := p.runCommand(name, match, ctx)
		p.currentRequestID = old
		p.mu.Unlock()
		oc := hookOutcome{PluginID: p.ID, Handled: true}
		if err != nil {
			oc.Error = err.Error()
			summary.Errored++
		} else {
			oc.Result = res
		}
		summary.Subscribed++
		all = append(all, oc)
	}
	return all, summary
}

// dispatchCommandByRegex 用文本匹配各插件指令正则（command 名称未命中的兜底）。
func (e *engine) dispatchCommandByRegex(text string, _ []string, ctx map[string]any, requestID string) ([]hookOutcome, dispatchSummary) {
	summary := dispatchSummary{AllowContinue: true}
	all := []hookOutcome{}
	e.mu.RLock()
	var candidates []*plugin
	for _, p := range e.plugins {
		if len(p.commands) > 0 {
			candidates = append(candidates, p)
		}
	}
	e.mu.RUnlock()
	for _, p := range candidates {
		p.mu.Lock()
		for _, cd := range p.commands {
			reg, err := regexp.Compile(cd.pattern)
			if err != nil || reg == nil {
				continue
			}
			matched := reg.FindStringSubmatch(text)
			if matched == nil {
				continue
			}
			old := p.currentRequestID
			p.currentRequestID = requestID
			res, rerr := p.callFn(cd.handler, matched, ctx)
			p.currentRequestID = old
			oc := hookOutcome{PluginID: p.ID, Handled: true}
			if rerr != nil {
				oc.Error = rerr.Error()
			} else {
				oc.Result = res
			}
			summary.Subscribed++
			all = append(all, oc)
			break
		}
		p.mu.Unlock()
	}
	return all, summary
}
