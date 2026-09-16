package YaraLTP

// ==== 插件核心 API：logger / event / hook / eventHandler / command / tool / api / llmProvider / send ====

import (
	"fmt"
	"strings"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

// bindLogger 注入 yara.logger。
func bindLogger(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)
	objSetFn(o, "info", func(call goja.FunctionCall) goja.Value {
		LoggerGeneral.Info(ServiceName, "%s", logMessage(call))
		return goja.Undefined()
	})
	objSetFn(o, "warn", func(call goja.FunctionCall) goja.Value {
		LoggerGeneral.Warn(ServiceName, "%s", logMessage(call))
		return goja.Undefined()
	})
	objSetFn(o, "error", func(call goja.FunctionCall) goja.Value {
		LoggerGeneral.Error(ServiceName, "%s", logMessage(call))
		return goja.Undefined()
	})
	objSetFn(o, "debug", func(call goja.FunctionCall) goja.Value {
		LoggerGeneral.Info(ServiceName, "[debug] %s", logMessage(call))
		return goja.Undefined()
	})
	parent.Set("logger", o)
}

// logMessage 组装 logger 消息：仅当存在后续参数时按 fmt 占位符插值，
// 否则把首个参数当作纯文本原样输出，避免含 % 的 URL/JSON 等被 Sprintf 破坏。
func logMessage(call goja.FunctionCall) string {
	msg := argString(call, 0)
	if len(call.Arguments) <= 1 {
		return msg
	}
	args := make([]any, 0, len(call.Arguments)-1)
	for i := 1; i < len(call.Arguments); i++ {
		args = append(args, argExport(call, i))
	}
	return fmt.Sprintf(msg, args...)
}

// bindEvent 注入 yara.event（订阅/发布）。
func bindEvent(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "subscribe", func(call goja.FunctionCall) goja.Value {
		topic := argString(call, 0)
		cb := call.Argument(1)
		if topic == "" || !isJSFunc(cb) {
			return goja.Undefined()
		}
		p.events[topic] = append(p.events[topic], &eventSub{handler: cb})
		return goja.Undefined()
	})
	objSetFn(o, "publish", func(call goja.FunctionCall) goja.Value {
		topic := argString(call, 0)
		payload := argExport(call, 1)
		// 调用方已持有插件锁（publish 多发生于插件回调内）
		p.fireEventLocal(topic, payload)
		return goja.Undefined()
	})
	parent.Set("event", o)
}

// bindHook 注入 yara.hook（支持 type,handler,options / type,options,handler 两种写法）。
func bindHook(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		hookType := argString(call, 0)
		var handler goja.Value
		opts := map[string]any{}
		if isJSFunc(call.Argument(1)) {
			handler = call.Argument(1)
			opts = argMap(call, 2)
		} else {
			opts = argMap(call, 1)
			handler = call.Argument(2)
		}
		if hookType == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.hooks[hookType] = append(p.hooks[hookType], &hookSub{
			hookType:    hookType,
			mode:        mapGetStr(opts, "mode"),
			order:       mapGetStr(opts, "order"),
			errorPolicy: mapGetStr(opts, "errorPolicy"),
			timeoutMs:   toInt64(mapGet(opts, "timeoutMs")),
			handler:     handler,
		})
		return goja.Undefined()
	})
	parent.Set("hook", o)
}

// bindEventHandler 注入 yara.eventHandler。
func bindEventHandler(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		name := argString(call, 0)
		eventType := argString(call, 1)
		handler := call.Argument(2)
		opts := argMap(call, 3)
		if name == "" || eventType == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.events[eventType] = append(p.events[eventType], &eventSub{
			name:             name,
			weight:           int(toInt64(mapGet(opts, "weight"))),
			interceptMessage: toBool(mapGet(opts, "interceptMessage")),
			handler:          handler,
		})
		return goja.Undefined()
	})
	parent.Set("eventHandler", o)
}

// bindCommand 注入 yara.command。
func bindCommand(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		name := argString(call, 0)
		pattern := argString(call, 1)
		handler := call.Argument(2)
		opts := argMap(call, 3)
		if name == "" || pattern == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		cd := &commandDef{name: name, pattern: pattern, handler: handler, aliases: mapGetStrSlice(opts, "aliases")}
		p.commands[name] = cd
		for _, a := range cd.aliases {
			if a != "" {
				p.commands[a] = cd
			}
		}
		return goja.Undefined()
	})
	parent.Set("command", o)
}

// bindTool 注入 yara.tool（register / registerAutonomous / getDefinitions）。
func bindTool(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		name := argString(call, 0)
		def := argMap(call, 1)
		handler := call.Argument(2)
		if name == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.tools[name] = toolDefFromReg(name, def, handler, "agent")
		p.appendToolOrder(name)
		return goja.Undefined()
	})
	objSetFn(o, "registerAutonomous", func(call goja.FunctionCall) goja.Value {
		def := argMap(call, 0)
		name := mapGetStr(def, "name")
		var handler goja.Value
		a0 := call.Argument(0)
		if !goja.IsNull(a0) && !goja.IsUndefined(a0) {
			handler = a0.ToObject(vm).Get("handler")
		}
		if name == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.tools[name] = &toolDef{
			name:        name,
			description: mapGetStr(def, "description"),
			hookType:    mapGetStr(def, "hookType"),
			pattern:     mapGetStr(def, "pattern"),
			toolType:    "autonomous",
			visibility:  "hidden",
			async:       toBool(mapGet(def, "async")),
			handler:     handler,
		}
		p.appendToolOrder(name)
		return goja.Undefined()
	})
	objSetFn(o, "getDefinitions", func(call goja.FunctionCall) goja.Value {
		defs := make([]map[string]any, 0, len(p.tools))
		for _, t := range p.tools {
			defs = append(defs, toolDefToMap(t))
		}
		return vm.ToValue(defs)
	})
	parent.Set("tool", o)
}

// bindApi 注入 yara.api（跨插件调用由管理器解析分段名定位目标插件）。
func bindApi(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		name := argString(call, 0)
		handler := call.Argument(1)
		opts := argMap(call, 2)
		if name == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.apis[name] = &apiDef{
			name:        name,
			description: mapGetStr(opts, "description"),
			version:     mapGetStr(opts, "version"),
			public:      toBool(mapGet(opts, "public")),
			handler:     handler,
		}
		return goja.Undefined()
	})
	objSetFn(o, "call", func(call goja.FunctionCall) goja.Value {
		qualified := argString(call, 0)
		params := argMap(call, 1)
		res, err := Engine.callCrossPlugin(p, qualified, params)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(res)
	})
	parent.Set("api", o)
}

// bindLLMProvider 注入 yara.llmProvider。
func bindLLMProvider(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "register", func(call goja.FunctionCall) goja.Value {
		clientType := argString(call, 0)
		handler := call.Argument(1)
		opts := argMap(call, 2)
		if clientType == "" || !isJSFunc(handler) {
			return goja.Undefined()
		}
		p.llmProviders[clientType] = &llmProvider{
			name:       mapGetStr(opts, "name"),
			clientType: clientType,
			handler:    handler,
		}
		return goja.Undefined()
	})
	parent.Set("llmProvider", o)
}

// bindSend 注入 yara.send（text 返回成功判定，单播/广播送达策略由总线依据 request_id 决定）。
func bindSend(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "text", func(call goja.FunctionCall) goja.Value {
		p.emitSend("text", argString(call, 0), argString(call, 1), "", "", "")
		return vm.ToValue(true)
	})
	objSetFn(o, "image", func(call goja.FunctionCall) goja.Value {
		p.emitSend("image", argString(call, 0), "", argString(call, 1), "", "")
		return vm.ToValue(true)
	})
	objSetFn(o, "emoji", func(call goja.FunctionCall) goja.Value {
		p.emitSend("emoji", argString(call, 0), "", "", argString(call, 1), "")
		return vm.ToValue(true)
	})
	objSetFn(o, "hybrid", func(call goja.FunctionCall) goja.Value {
		p.emitSend("hybrid", argString(call, 0), "", "", "", argExport(call, 1))
		return vm.ToValue(true)
	})
	parent.Set("send", o)
}

// emitSend 构造 send 总线消息并广播（携带当前请求 ID 以支持单播回执）。
func (p *plugin) emitSend(kind, groupID, content, image, emoji string, segments any) {
	emitBus(sendMessage{
		Type:      "ltp3/send",
		RequestID: p.currentRequestID,
		PluginID:  p.ID,
		Kind:      kind,
		GroupID:   groupID,
		Content:   content,
		Image:     image,
		Emoji:     emoji,
		Segments:  toSegments(segments),
		Success:   true,
	})
}

// fireEventLocal 在插件内派发事件到订阅者（调用方必须已持有插件锁）。
func (p *plugin) fireEventLocal(topic string, payload any) {
	subs := p.events[topic]
	if len(subs) == 0 {
		return
	}
	for _, sub := range subs {
		if sub.handler == nil {
			continue
		}
		data := payload
		if data == nil {
			data = map[string]any{}
		}
		if _, err := p.callFn(sub.handler, data); err != nil {
			LoggerGeneral.Warn(ServiceName, "插件 %s 事件订阅 %s 执行异常: %v", p.ID, topic, err)
		}
	}
}

// runHook 在插件内执行指定挂钩点的全部订阅者（调用方已持有插件锁），返回各结果。
func (p *plugin) runHook(hookType string, message any, ctx map[string]any) []hookOutcome {
	subs := p.hooks[hookType]
	out := make([]hookOutcome, 0, len(subs))
	for _, sub := range subs {
		if sub == nil || sub.handler == nil {
			continue
		}
		event := map[string]any{
			"type":    hookType,
			"message": message,
			"context": ctx,
		}
		res, err := p.callFn(sub.handler, event)
		hc := hookOutcome{PluginID: p.ID, Handled: true}
		if err != nil {
			hc.Error = err.Error()
		} else {
			hc.Result = res
		}
		out = append(out, hc)
	}
	return out
}

// runCommand 在插件内执行指定指令处理函数（调用方已持有插件锁）。
func (p *plugin) runCommand(name string, match []string, ctx map[string]any) (any, error) {
	cd, ok := p.commands[name]
	if !ok || cd == nil || cd.handler == nil {
		return nil, fmt.Errorf("指令 %s 未注册", name)
	}
	return p.callFn(cd.handler, match, ctx)
}

// toolDefFromReg 把 register 传入的定义结构转为 toolDef。
func toolDefFromReg(name string, def map[string]any, handler goja.Value, defToolType string) *toolDef {
	td := &toolDef{
		name:                name,
		description:         mapGetStr(def, "description"),
		briefDescription:    mapGetStr(def, "briefDescription"),
		detailedDescription: mapGetStr(def, "detailedDescription"),
		visibility:          mapGetStr(def, "visibility"),
		toolType:            mapGetStr(def, "toolType"),
		async:               toBool(mapGet(def, "async")),
		timeoutSeconds:      toInt64(mapGet(def, "timeoutSeconds")),
		handler:             handler,
	}
	if td.toolType == "" {
		td.toolType = defToolType
	}
	if td.visibility == "" {
		td.visibility = "visible"
	}
	if params, ok := def["parameters"].([]any); ok {
		for _, raw := range params {
			if m, is := raw.(map[string]any); is {
				td.parameters = append(td.parameters, toolParam{
					Name:        mapGetStr(m, "name"),
					Type:        mapGetStr(m, "type"),
					Description: mapGetStr(m, "description"),
					Required:    toBool(mapGet(m, "required")),
					Default:     mapGet(m, "default"),
					EnumValues:  mapGetStrSlice(m, "enumValues"),
				})
			}
		}
	}
	return td
}

// toolDefToMap 工具定义导出为 JS 可见对象。
func toolDefToMap(t *toolDef) map[string]any {
	if t == nil {
		return nil
	}
	params := make([]map[string]any, 0, len(t.parameters))
	for _, p := range t.parameters {
		params = append(params, map[string]any{
			"name":        p.Name,
			"type":        p.Type,
			"description": p.Description,
			"required":    p.Required,
			"default":     p.Default,
			"enumValues":  p.EnumValues,
		})
	}
	return map[string]any{
		"name":               t.name,
		"description":        t.description,
		"briefDescription":   t.briefDescription,
		"detailedDescription": t.detailedDescription,
		"visibility":         t.visibility,
		"toolType":           t.toolType,
		"timeoutSeconds":     t.timeoutSeconds,
		"async":              t.async,
		"parameters":         params,
	}
}

// appendToolOrder 记录工具注册顺序（getDefinitions 保序）。
func (p *plugin) appendToolOrder(name string) {
	for _, n := range p.toolRegOrder {
		if n == name {
			return
		}
	}
	p.toolRegOrder = append(p.toolRegOrder, name)
}

// isJSFunc 判断 goja.Value 是否可断言为函数。
func isJSFunc(v goja.Value) bool {
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	_, ok := goja.AssertFunction(v)
	return ok
}

// mapGet 读取 map 键。
func mapGet(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

// toInt64 把任意值安全转为 int64。
func toInt64(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case float64:
		return int64(t)
	case int:
		return int64(t)
	}
	return 0
}

// toBool 把任意值安全转为 bool。
func toBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	}
	return false
}

// toSegments 把 hybrid 参数转为 []any（保持原数组）。
func toSegments(v any) []any {
	if arr, ok := v.([]any); ok {
		return arr
	}
	return nil
}