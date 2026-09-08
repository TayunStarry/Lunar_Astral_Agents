package StarLTP

// ==== 单插件 goja 沙箱：加载 / 卸载 / 回调执行 / 事件派发 ====

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// callAwaitTimeout 等待异步导出函数（Promise）敲定的超时。
const callAwaitTimeout = 90 * time.Second

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
	}
	_ = os.MkdirAll(p.DataDir, 0755)
	// 启动插件独立事件循环（goja.New + 自带定时器）
	p.loop = eventloop.NewEventLoop()
	p.loop.Start()
	return p
}

// load 创建插件沙箱、绑定 engine.*、执行 execute.js、调用 onLoad。
func (p *plugin) load() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded {
		return nil
	}

	// 0. 读取配置（config.yaml 只读解析；失败仅告警，不阻断加载）
	if raw, cerr := os.ReadFile(p.ConfigPath); cerr == nil {
		if cfg, perr := parseYAML(string(raw)); perr == nil {
			p.config = cfg
		} else {
			LoggerGeneral.Warn(ServiceName, "插件 %s config.yaml 解析失败: %v", p.ID, perr)
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

	// 3. 在插件事件循环内绑定 engine.* 并执行脚本（goja 只在 loop 线程内访问）
	var runErr error
	loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("engine", bindEngine(vm, p))
		// 沙箱网络：fetch / WebSocket 全局（allow-network 门控）
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
	})
	if !loopOK {
		p.loadErr = "插件事件循环已终止，无法加载"
		return fmt.Errorf("%s", p.loadErr)
	}
	if runErr != nil {
		p.loadErr = fmt.Sprintf("execute.js 执行失败: %v", runErr)
		return fmt.Errorf("%s", p.loadErr)
	}

	// 4. onLoad
	if p.onLoad != nil {
		if _, cerr := p.callFn(p.onLoad); cerr != nil {
			LoggerGeneral.Warn(ServiceName, "插件 %s onLoad 执行异常: %v", p.ID, cerr)
		}
	}

	p.loaded = true
	p.loadErr = ""
	LoggerGeneral.Info(ServiceName, "LTP9 插件已加载: %s (%s)", p.ID, p.Root)
	return nil
}

// unload 调用 onUnload 并停止插件事件循环。
func (p *plugin) unload() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.onUnload != nil {
		p.loop.RunOnLoop(func(vm *goja.Runtime) {
			if f, ok := goja.AssertFunction(p.onUnload); ok {
				f(goja.Undefined())
			}
		})
	}
	p.loop.Stop()
	p.loaded = false
	p.onLoad, p.onUnload = nil, nil
	p.config = nil
	p.events = map[string][]*eventSub{}
	p.exports = map[string]jsFunc{}
	p.frontSignal = nil
	p.granted = nil
	LoggerGeneral.Info(ServiceName, "LTP9 插件已卸载: %s", p.ID)
}

// callFn 在插件事件循环内调用一个 JS 函数并同步返回其结果（同步阻塞，不做异步承诺）。
// 事件订阅回调与导出函数均为同步函数，调用方在此阻塞直到回调返回并拿到普通值；
// 若回调返回的 Promise 已敲定，取其 Result 返回；仍未敲定则按空结果兜底。
// 返回后调用方才继续。
func (p *plugin) callFn(fn jsFunc, args ...any) (any, error) {
	type result struct {
		out any
		err error
	}
	done := make(chan result, 1)
	p.loop.RunOnLoop(func(vm *goja.Runtime) {
		f, ok := goja.AssertFunction(fn)
		if !ok {
			done <- result{err: fmt.Errorf("非函数对象")}
			return
		}
		callArgs := make([]goja.Value, 0, len(args))
		for _, a := range args {
			callArgs = append(callArgs, vm.ToValue(a))
		}
		v, err := f(goja.Undefined(), callArgs...)
		if err != nil {
			if ex, ok := err.(*goja.Exception); ok {
				done <- result{err: fmt.Errorf("%v", ex.Value())}
			} else {
				done <- result{err: err}
			}
			return
		}
		// Promise 分支（同步阻塞语义；本包插件均已同步，通常不进入）
		if pm, ok := v.Export().(*goja.Promise); ok {
			if pm.State() == goja.PromiseStateRejected {
				done <- result{err: fmt.Errorf("%s", pm.Result().String())}
				return
			}
			if pm.State() == goja.PromiseStateFulfilled {
				rv := pm.Result()
				if rv != nil && !goja.IsUndefined(rv) && !goja.IsNull(rv) {
					done <- result{out: rv.Export()}
					return
				}
			}
			done <- result{} // pending 兜底
			return
		}
		done <- result{out: v.Export()}
	})
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res.out, nil
	case <-time.After(callAwaitTimeout):
		return nil, fmt.Errorf("函数调用超时（Promise 未在 %s 内敲定）", callAwaitTimeout)
	}
}

// isFunc 判断 goja 值是否为函数。
func isFunc(v goja.Value) bool {
	_, ok := goja.AssertFunction(v)
	return ok
}

// registerEvent 记录一次事件订阅，返回订阅 id（单调递增，供 unsubscribe 精确退订）。
// priority 为订阅优先级（0 最高）；priorityUnset 表示未设置，按时间顺序排在设置者之后。
func (p *plugin) registerEvent(topic string, cb jsFunc, priority int) int {
	p.subSeq++
	id := p.subSeq
	p.events[topic] = append(p.events[topic], &eventSub{topic: topic, orderID: id, handler: cb, priority: priority})
	return id
}

// subOrderLess 事件订阅器派发顺序：优先按优先级升序（0 最高）；同优先级按订阅时间（orderID）升序；
// 未设置优先级的订阅视作最大优先级，排在所有设置者之后并保持彼此的时间顺序。
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
// 回调返回普通对象（非 Promise）；引擎同步阻塞拿到回调结果：
//   - { intercept:true }   → 本次事件已被消费，短路该插件后续订阅器
//   - { modifiedData: X }  → 用 X 改写事件负载，供本插件后续订阅器读取
//   - { cancel:true }      → 撤回本次事件，停止派发
// 注意：执行回调（callFn）时不得持有 p.mu，否则回调内的 engine.signal.all 会去 Lock 同一把锁而自锁。
func (p *plugin) fireEvent(topic string, payload any) []Outcome {
	p.mu.Lock()
	subs := append([]*eventSub(nil), p.events[topic]...)
	p.mu.Unlock()
	// 按优先级（高者先）与时间顺序排序后派发，保证拦截参数按期望顺序命中
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
				// 撤回：停止派发（含后续订阅器），事件不再扩散
				if m["cancel"] == true {
					out = append(out, oc)
					break
				}
				// 改写：用返回的 modifiedData 替换后续订阅器读取的负载
				if v, has := m["modifiedData"]; has && v != nil {
					cur = v
				}
				// 拦截：本次事件已消费，短路后续订阅器
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
// 注意：取到 handler 后须先释放 p.mu 再 callFn，避免导出函数内 engine.signal.all 自锁。
func (p *plugin) callExport(fnName string, args []any) (any, error) {
	p.mu.Lock()
	h, ok := p.exports[fnName]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s 包拒绝响应：未导出函数 %s", p.ID, fnName)
	}
	return p.callFn(h, args)
}