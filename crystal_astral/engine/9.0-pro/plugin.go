package ltp9

// ==== 单插件 goja 沙箱：加载 / 卸载 / 回调执行 / 事件派发 ====

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// callAwaitTimeout 等待异步导出函数（Promise）敲定的超时。
const callAwaitTimeout = 90 * time.Second

// pluginLoadTimeout 等待 execute.js 顶层执行完成的超时：goja 同步执行无法被打断。
const pluginLoadTimeout = 30 * time.Second

// unloadGraceTimeout 卸载时等待事件循环响应（探针回调）的宽限。
const unloadGraceTimeout = 5 * time.Second

// callResult 一次回调执行的返回载体：out 为普通结果，err 为错误。
type callResult struct {
	out any
	err error
}

// newPlugin 依据包目录构造插件实例，并为其创建独立事件循环（goja.New + 自带定时器）。
func newPlugin(dir, root, id, title string) *plugin {
	p := &plugin{
		ID:         id,
		DirName:    dir,
		Title:      title,
		Root:       root,
		MainPath:   filepath.Join(root, DefaultMain),
		ConfigPath: filepath.Join(root, DefaultConfigFile),
		DataDir:    filepath.Join(root, DataDirName),
		KeyPath:    filepath.Join(root, KeyFileName),
		events:     map[string][]*eventSub{},
		exports:    map[string]jsFunc{},
		commands:   map[string]*commandEntry{},
		tools:      map[string]*toolEntry{},
		asyncTasks: map[int]*asyncTask{},
	}
	_ = os.MkdirAll(p.DataDir, 0755)
	// 启动插件独立事件循环（goja.New + 自带定时器）
	p.loop = eventloop.NewEventLoop()
	p.loop.Start()
	return p
}

// load 创建插件沙箱、绑定 engine.*、执行 execute.js、调用 onLoad。
func (p *plugin) load() error {
	if p.loaded {
		return nil
	}

	// 0. 读取配置（config.yaml 只读解析；失败仅告警，不阻断加载）
	if raw, cerr := os.ReadFile(p.ConfigPath); cerr == nil {
		if cfg, perr := parseYAML(string(raw)); perr == nil {
			p.config = cfg
		} else {
			LoggerGeneral.Warn(ServiceName, "插件 config.yaml 解析失败 plugin=%s err=%v", p.ID, perr)
		}
	}

	// 1. 权限：permissions.key 代码哈希解密 → allow-* 清单
	p.granted = p.verifyPermissions()

	// 2. 读取主脚本
	code, err := os.ReadFile(p.MainPath)
	if err != nil {
		p.loadErr = fmt.Sprintf("读取 execute.js 失败: %v", err)
		return fmt.Errorf("%s", p.loadErr)
	}

	// 3. 在插件事件循环内绑定 engine.* 并执行脚本
	var runErr error
	loaded := make(chan struct{})
	loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() { close(loaded) }()
		vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", false))
		vm.Set("console", bindConsole(vm))
		vm.Set("engine", bindEngine(vm, p))
		if p.hasPerm("allow-network") {
			bindNetwork(vm, p)
		}
		_, runErr = vm.RunString(string(code))
		if runErr != nil {
			return
		}
		if v := vm.Get("onLoad"); isFunc(v) {
			p.onLoad = v
		}
		if v := vm.Get("onUnload"); isFunc(v) {
			p.onUnload = v
		}
		if v := vm.Get("onConfigUpdate"); isFunc(v) {
			p.onConfigUpdate = v
		}
	})
	if !loopOK {
		p.loadErr = "插件事件循环已终止，无法加载"
		return fmt.Errorf("%s", p.loadErr)
	}
	select {
	case <-loaded:
	case <-time.After(pluginLoadTimeout):
		p.loadErr = fmt.Sprintf("execute.js 顶层执行超过 %s 未完成（可能存在死循环），加载中止", pluginLoadTimeout)
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		LoggerGeneral.Error(ServiceName, "LTP9 插件加载超时，dump goroutine 栈 plugin=%s bytes=%d stack=%s", p.ID, n, string(buf[:n]))
		return fmt.Errorf("%s", p.loadErr)
	}
	if runErr != nil {
		p.loadErr = fmt.Sprintf("execute.js 执行失败: %v", runErr)
		return fmt.Errorf("%s", p.loadErr)
	}

	// 4. onLoad
	if p.onLoad != nil {
		if _, cerr := p.callFn(p.onLoad); cerr != nil {
			LoggerGeneral.Warn(ServiceName, "插件 onLoad 执行异常 plugin=%s err=%v", p.ID, cerr)
		}
	}

	p.loaded = true
	p.loadErr = ""
	LoggerGeneral.Info(ServiceName, "LTP9 插件已加载 plugin=%s dir=%s", p.ID, p.Root)
	return nil
}

// unload 调用 onUnload 并终止插件事件循环。
func (p *plugin) unload() {
	if p == nil {
		return
	}
	canary := make(chan struct{})
	ok := p.loop.RunOnLoop(func(vm *goja.Runtime) { close(canary) })
	if ok {
		select {
		case <-canary:
			p.loop.RunOnLoop(func(vm *goja.Runtime) {
				if p.onUnload != nil {
					if f, cbOK := goja.AssertFunction(p.onUnload); cbOK {
						f(goja.Undefined())
					}
				}
			})
			p.loop.Terminate()
		case <-time.After(unloadGraceTimeout):
			p.loop.StopNoWait()
		}
	} else {
		p.loop.Terminate()
	}
	p.mu.Lock()
	p.loaded = false
	p.onLoad, p.onUnload, p.onConfigUpdate = nil, nil, nil
	p.config = nil
	p.events = map[string][]*eventSub{}
	p.exports = map[string]jsFunc{}
	p.commands = map[string]*commandEntry{}
	p.tools = map[string]*toolEntry{}
	p.configSchema = nil
	p.asyncTasks = map[int]*asyncTask{}
	p.frontSignal = nil
	p.granted = nil
	p.mu.Unlock()
	LoggerGeneral.Info(ServiceName, "LTP9 插件已卸载 plugin=%s", p.ID)
}

// callFn 在插件事件循环内调用一个 JS 函数并同步返回其结果。
func (p *plugin) callFn(fn jsFunc, args ...any) (any, error) {
	done := make(chan callResult, 1)
	loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
		f, ok := goja.AssertFunction(fn)
		if !ok {
			done <- callResult{err: fmt.Errorf("非函数对象")}
			return
		}
		callArgs := make([]goja.Value, 0, len(args))
		for _, a := range args {
			callArgs = append(callArgs, vm.ToValue(a))
		}
		v, err := f(goja.Undefined(), callArgs...)
		if err != nil {
			if ex, ok := err.(*goja.Exception); ok {
				done <- callResult{err: fmt.Errorf("%v", ex.Value())}
			} else {
				done <- callResult{err: err}
			}
			return
		}
		pm, ok := v.Export().(*goja.Promise)
		if !ok {
			done <- callResult{out: v.Export()}
			return
		}
		if pm.State() != goja.PromiseStatePending {
			done <- settlePromise(pm)
			return
		}
		go p.awaitPromise(pm, done)
	})
	if !loopOK {
		return nil, fmt.Errorf("插件事件循环已终止，无法执行回调")
	}
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res.out, nil
	case <-time.After(callAwaitTimeout + 10*time.Second):
		return nil, fmt.Errorf("函数调用总超时（含 eventloop 调度）")
	}
}

// settlePromise 从已敲定的 Promise 提取结果（仅在插件 loop 线程内调用）。
func settlePromise(pm *goja.Promise) callResult {
	if pm.State() == goja.PromiseStateRejected {
		return callResult{err: fmt.Errorf("%s", pm.Result().String())}
	}
	if pm.State() == goja.PromiseStateFulfilled {
		rv := pm.Result()
		if rv != nil && !goja.IsUndefined(rv) && !goja.IsNull(rv) {
			return callResult{out: rv.Export()}
		}
		return callResult{out: nil}
	}
	return callResult{}
}

// awaitPromise 在插件事件循环之外等待 pending Promise 敲定。
func (p *plugin) awaitPromise(pm *goja.Promise, done chan callResult) {
	guard := time.After(callAwaitTimeout)
	for {
		select {
		case <-guard:
			done <- callResult{err: fmt.Errorf("Promise 未在 %s 内敲定", callAwaitTimeout)}
			return
		case <-time.After(5 * time.Millisecond):
		}
		var r callResult
		settled := false
		loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
			if pm.State() == goja.PromiseStatePending {
				return
			}
			settled = true
			r = settlePromise(pm)
		})
		if !loopOK {
			done <- callResult{err: fmt.Errorf("插件事件循环已终止，Promise 无法敲定")}
			return
		}
		if settled {
			done <- r
			return
		}
	}
}

// isFunc 判断 goja 值是否为函数。
func isFunc(v goja.Value) bool {
	_, ok := goja.AssertFunction(v)
	return ok
}

// registerEvent 记录一次事件订阅，返回订阅 id。
func (p *plugin) registerEvent(topic string, cb jsFunc, priority int) int {
	p.subSeq++
	id := p.subSeq
	p.events[topic] = append(p.events[topic], &eventSub{topic: topic, orderID: id, handler: cb, priority: priority})
	return id
}

// subOrderLess 事件订阅器派发顺序：优先按优先级升序；同优先级按订阅时间；未设置优先级的排在设置者之后。
func subOrderLess(a, b *eventSub) bool {
	pa, pb := a.priority, b.priority
	if pa == priorityUnset {
		pa = 1 << 30
	}
	if pb == priorityUnset {
		pb = 1 << 30
	}
	if pa != pb {
		return pa < pb
	}
	return a.orderID < b.orderID
}

// removeEvent 按订阅 id 移除事件订阅。
func (p *plugin) removeEvent(topic string, id int) {
	list := p.events[topic]
	if len(list) == 0 {
		return
	}
	kept := list[:0]
	for _, s := range list {
		if s.orderID != id {
			kept = append(kept, s)
		}
	}
	p.events[topic] = kept
}

// fireEvent 派发事件：按注册顺序同步调用订阅回调，支持拦截/改写/撤回。
func (p *plugin) fireEvent(topic string, payload any) []Outcome {
	p.mu.Lock()
	loaded := p.loaded
	subs := append([]*eventSub(nil), p.events[topic]...)
	p.mu.Unlock()
	if !loaded {
		return nil
	}
	sort.SliceStable(subs, func(i, j int) bool { return subOrderLess(subs[i], subs[j]) })
	out := make([]Outcome, 0, len(subs))
	cur := payload
	for _, s := range subs {
		oc := Outcome{PluginID: p.ID, Handled: true}
		if res, err := p.callFn(s.handler, map[string]any{"type": topic, "payload": cur}); err != nil {
			oc.Error = err.Error()
		} else {
			oc.Result = res
			if m, ok := res.(map[string]any); ok {
				if m["cancel"] == true {
					out = append(out, oc)
					break
				}
				if v, has := m["modifiedData"]; has && v != nil {
					cur = v
				}
				if m["intercept"] == true {
					out = append(out, oc)
					break
				}
			}
		}
		out = append(out, oc)
	}
	return out
}

// callExport 调用插件导出的函数（engine.call 目标）。
func (p *plugin) callExport(fnName string, args []any) (any, error) {
	p.mu.Lock()
	h, ok := p.exports[fnName]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s 包拒绝响应：未导出函数 %s", p.ID, fnName)
	}
	return p.callFn(h, args...)
}

// registerTool 记录一次工具注册（engine.tool.register），覆盖同名工具。
func (p *plugin) registerTool(t *toolEntry) {
	p.mu.Lock()
	p.tools[t.name] = t
	p.mu.Unlock()
}

// registerConfigSchema 记录一次配置 schema 声明（engine.config.registerSchema），多次声明追加合并（按节名覆盖）。
func (p *plugin) registerConfigSchema(sections []ConfigSectionDef) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(sections) == 0 {
		return
	}
	merged := make([]ConfigSectionDef, 0, len(p.configSchema)+len(sections))
	seen := map[string]int{}
	for _, s := range p.configSchema {
		seen[s.Name] = len(merged)
		merged = append(merged, s)
	}
	for _, s := range sections {
		if idx, ok := seen[s.Name]; ok {
			merged[idx] = s
			continue
		}
		seen[s.Name] = len(merged)
		merged = append(merged, s)
	}
	p.configSchema = merged
}

// callTool 调用插件注册的工具（CallTool 目标）。
func (p *plugin) callTool(name string, args map[string]any, context map[string]any) (any, error) {
	p.mu.Lock()
	t, ok := p.tools[name]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s 包未注册工具 %s", p.ID, name)
	}
	return p.callFn(t.handler, args, context)
}

// fireConfigUpdate 配置回写后触发 onConfigUpdate(scope, config, version) 生命周期回调。
func (p *plugin) fireConfigUpdate(scope string, config any, version string) {
	if p == nil || p.onConfigUpdate == nil || p.loop == nil {
		return
	}
	cb := p.onConfigUpdate
	p.loop.RunOnLoop(func(vm *goja.Runtime) {
		if f, ok := goja.AssertFunction(cb); ok {
			f(goja.Undefined(), vm.ToValue(scope), vm.ToValue(config), vm.ToValue(version))
		}
	})
}