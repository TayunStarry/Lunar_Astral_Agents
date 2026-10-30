package runtime

// ==== 私有 goja 沙箱：加载 / 卸载 / 回调执行 ====

import (
	"fmt"
	"os"
	"sort"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// callFnTimeout 单次同步回调的总等待上限（含 eventloop 调度）。
const callFnTimeout = 90 * time.Second

// scriptLoadTimeout 等待脚本顶层执行完成的超时：goja 同步执行无法被打断，
// 死循环脚本不再无限阻塞调用方（循环本身成为僵尸，由 unload 的 StopNoWait 兜底）。
const scriptLoadTimeout = 30 * time.Second

// unloadGraceTimeout 卸载时等待事件循环响应的宽限：超时视为循环卡死，改用 StopNoWait。
const unloadGraceTimeout = 5 * time.Second

// callResult 一次回调执行的返回载体：out 为普通结果，err 为错误。
type callResult struct {
	out any
	err error
}

// newPrivate 创建一个私有运行时实例（独立事件循环 + 隔离数据目录）。
func newPrivate(id, title string) *plugin {
	p := &plugin{
		ID:       id,
		Title:    title,
		DataDir:  privateDataDir(id),
		events:   map[string][]*eventSub{},
		exports:  map[string]jsFunc{},
		commands: map[string]*commandEntry{},
	}
	_ = os.MkdirAll(p.DataDir, 0o755)
	p.loop = eventloop.NewEventLoop()
	p.loop.Start()
	return p
}

// loadSource 在私有沙箱内绑定全局并执行脚本源码。
// 注意：加载全程不持有 p.mu。脚本在事件循环内执行，注册导出时会自己去拿 p.mu；
// 若此处持 p.mu 并等待脚本完成（<-loaded），会与脚本的 p.mu 申请构成循环死锁。
func (p *plugin) loadSource(code string) error {
	if p.loaded {
		return nil
	}

	// 沙箱日志统一走 logger（经日志管线进入「系统日志 · 生活」分类）。
	var runErr error
	loaded := make(chan struct{})
	loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() { close(loaded) }()
		// 结构体→JS 对象按 json 标签映射字段名（rwResult 的 success/text/error 等小写契约）
		vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", false))
		bindSandbox(vm, p)
		_, runErr = vm.RunString(code)
	})
	if !loopOK {
		p.loadErr = "运行时事件循环已终止，无法加载"
		return fmt.Errorf("%s", p.loadErr)
	}
	select {
	case <-loaded:
	case <-time.After(scriptLoadTimeout):
		p.loadErr = fmt.Sprintf("脚本顶层执行超过 %s 未完成（可能存在死循环），加载中止", scriptLoadTimeout)
		return fmt.Errorf("%s", p.loadErr)
	}
	if runErr != nil {
		p.loadErr = fmt.Sprintf("脚本执行失败: %v", runErr)
		return fmt.Errorf("%s", p.loadErr)
	}

	p.loaded = true
	p.loadErr = ""
	logger.Info(ServiceName, "js 运行时已加载: %s", p.ID)
	return nil
}

// unload 终止事件循环并清理全部运行时状态。
// 注意：全程不持有 p.mu —— goja_nodejs 的 Terminate/Stop 会同步等待事件循环退出，
// 若此刻循环内正在执行的 JS 回调申请 p.mu（绑定层），持锁等待即构成死锁。
func (p *plugin) unload() {
	if p == nil {
		return
	}
	// 探针：确认事件循环仍可调度（被死循环卡死的循环无法在宽限期内响应）
	canary := make(chan struct{})
	ok := p.loop.RunOnLoop(func(vm *goja.Runtime) { close(canary) })
	if ok {
		select {
		case <-canary:
			// 循环存活：Terminate —— 停止前会 runAux 清空待执行任务，
			// 并取消全部 setTimeout/setInterval（Stop 不清理定时器，会泄漏其 goroutine）
			p.loop.Terminate()
		case <-time.After(unloadGraceTimeout):
			// 循环已被 JS 死循环卡死：仅请求停止不等待（定时器 goroutine 泄漏优于卸载方挂死）
			p.loop.StopNoWait()
		}
	} else {
		p.loop.Terminate()
	}
	p.mu.Lock()
	p.loaded = false
	p.events = map[string][]*eventSub{}
	p.exports = map[string]jsFunc{}
	p.signalSubs = nil
	p.mu.Unlock()
	logger.Info(ServiceName, "js 运行时已卸载: %s", p.ID)
}

// callFn 在运行时事件循环内调用一个 JS 函数并同步返回其结果。
//
// 运行时只提供同步回调：导出函数必须直接返回普通值。
// 若回调返回 Promise（async 函数 / 未 await 的异步调用），直接报错——
// 需要异步的脚本应自行用定时器与回调组织，但「注册给宿主的函数」本身必须同步可得。
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
		if _, isPromise := v.Export().(*goja.Promise); isPromise {
			done <- callResult{err: fmt.Errorf("回调返回了 Promise：私有 js 运行时只支持同步回调，请自行在脚本内完成异步")}
			return
		}
		done <- callResult{out: v.Export()}
	})
	if !loopOK {
		return nil, fmt.Errorf("运行时事件循环已终止，无法执行回调")
	}
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res.out, nil
	case <-time.After(callFnTimeout):
		return nil, fmt.Errorf("函数调用总超时（含 eventloop 调度）")
	}
}

// isFunc 判断 goja 值是否为函数。
func isFunc(v goja.Value) bool {
	_, ok := goja.AssertFunction(v)
	return ok
}

// registerEvent 记录一次事件订阅，返回订阅 id（单调递增，供 unsubscribe 精确退订）。
func (p *plugin) registerEvent(topic string, cb jsFunc, priority int) int {
	p.subSeq++
	id := p.subSeq
	p.events[topic] = append(p.events[topic], &eventSub{topic: topic, orderID: id, handler: cb, priority: priority})
	return id
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

// exportNames 返回当前已导出的函数名（排序后）。
func (p *plugin) exportNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, 0, len(p.exports))
	for name := range p.exports {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// fireEvent 派发事件：按注册顺序同步调用订阅回调，支持拦截/改写/撤回。
// 回调返回普通对象（非 Promise）；宿主同步阻塞拿到回调结果：
//   - { intercept:true }   → 本次事件已被消费，短路该运行时后续订阅器
//   - { modifiedData: X }  → 用 X 改写事件负载，供本运行时后续订阅器读取
//   - { cancel:true }      → 撤回本次事件，停止派发
//
// 注意：执行回调（callFn）时不得持有 p.mu，否则回调内的 signal.all 会去 Lock 同一把锁而自锁。
func (p *plugin) fireEvent(topic string, payload any) {
	p.mu.Lock()
	loaded := p.loaded
	subs := append([]*eventSub(nil), p.events[topic]...)
	p.mu.Unlock()
	if !loaded {
		return
	}
	sort.SliceStable(subs, func(i, j int) bool { return subOrderLess(subs[i], subs[j]) })
	cur := payload
	for _, s := range subs {
		res, err := p.callFn(s.handler, map[string]any{"type": topic, "payload": cur})
		if err != nil {
			continue
		}
		if m, ok := res.(map[string]any); ok {
			if m["cancel"] == true {
				return
			}
			if v, has := m["modifiedData"]; has && v != nil {
				cur = v
			}
			if m["intercept"] == true {
				return
			}
		}
	}
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

// callExport 调用运行时的导出函数（callFunction 目标）。
// 注意：取到 handler 后须先释放 p.mu 再 callFn，避免导出函数内 signal.all 自锁。
func (p *plugin) callExport(fnName string, args []any) (any, error) {
	p.mu.Lock()
	h, ok := p.exports[fnName]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s 运行时未导出函数 %s", p.ID, fnName)
	}
	return p.callFn(h, args...)
}
