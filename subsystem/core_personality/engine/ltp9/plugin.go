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

// pluginLoadTimeout 等待 execute.js 顶层执行完成的超时：goja 同步执行无法被打断，
// 死循环脚本不再无限阻塞加载方（循环本身成为僵尸，由 unload 的 StopNoWait 兜底）。
const pluginLoadTimeout = 30 * time.Second

// unloadGraceTimeout 卸载时等待事件循环响应（探针回调）的宽限：超时视为循环卡死，改用 StopNoWait。
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
		asyncTasks: map[int]*asyncTask{},
	}
	_ = os.MkdirAll(p.DataDir, 0755)
	// 启动插件独立事件循环（goja.New + 自带定时器）
	p.loop = eventloop.NewEventLoop()
	p.loop.Start()
	return p
}

// load 创建插件沙箱、绑定 LTP9-Flash 顶层全局、执行 execute.js。
func (p *plugin) load() error {
	// 注意：加载全程不持有 p.mu。脚本在事件循环内执行，注册订阅/导出时会自己去拿 p.mu；
	// 若此处持 p.mu 并等待脚本完成（<-loaded），会与脚本的 p.mu 申请构成循环死锁。
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

	// 3. 在插件事件循环内绑定顶层全局并执行脚本（goja 只在 loop 线程内访问）
	//    注意：RunOnLoop 是非阻塞的，必须等脚本顶层执行完才能返回，
	//    否则 Init() 返回时订阅器/导出尚未注册，紧随其后的事件派发会漏注册（偶发须同步化）。
	var runErr error
	loaded := make(chan struct{})
	loopOK := p.loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() { close(loaded) }()
		// 结构体→JS 对象按 json 标签映射字段名（rwResult 的 success/text/error 等小写契约）
		vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", false))
		// LTP9-Flash 顶层全局（signal/file/memory/...，按 allow-* 注入）
		bindSandbox(vm, p)
		_, runErr = vm.RunString(string(code))
		if runErr != nil {
			return
		}
	})
	if !loopOK {
		p.loadErr = "插件事件循环已终止，无法加载"
		return fmt.Errorf("%s", p.loadErr)
	}
	// 同步等待脚本顶层执行完成（订阅器/导出注册就绪）；带超时防死循环脚本无限阻塞
	select {
	case <-loaded:
	case <-time.After(pluginLoadTimeout):
		p.loadErr = fmt.Sprintf("execute.js 顶层执行超过 %s 未完成（可能存在死循环），加载中止", pluginLoadTimeout)
		return fmt.Errorf("%s", p.loadErr)
	}
	if runErr != nil {
		p.loadErr = fmt.Sprintf("execute.js 执行失败: %v", runErr)
		return fmt.Errorf("%s", p.loadErr)
	}

	p.loaded = true
	p.loadErr = ""
	LoggerGeneral.Info(ServiceName, "LTP9 插件已加载: %s (%s)", p.ID, p.Root)
	return nil
}

// unload 终止插件事件循环并清理全部运行时状态。
// 注意：全程不持有 p.mu —— goja_nodejs 的 Terminate/Stop 会同步等待事件循环退出，
// 若此刻循环内正在执行的 JS 回调申请 p.mu（engine 绑定），持锁等待即构成死锁。
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
		// 循环已终止（重复卸载）：补一次 Terminate 清理残余定时器（对已终止循环立即返回）
		p.loop.Terminate()
	}
	p.mu.Lock()
	p.loaded = false
	p.config = nil
	p.events = map[string][]*eventSub{}
	p.exports = map[string]jsFunc{}
	p.commands = map[string]*commandEntry{}
	p.asyncTasks = map[int]*asyncTask{}
	p.signalSubs = nil
	p.granted = nil
	p.mu.Unlock()
	LoggerGeneral.Info(ServiceName, "LTP9 插件已卸载: %s", p.ID)
}

// callFn 在插件事件循环内调用一个 JS 函数并同步返回其结果。
// 若函数返回 Promise（async 函数 / await fetch/WebSocket），由后台 goroutine 通过反复
// 短回插件 loop 的方式轮询，期间 loop 保持空闲可继续处理网络回调与微任务，直至敲定或超时。
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

		// 非 Promise：直接返回导出结果
		pm, ok := v.Export().(*goja.Promise)
		if !ok {
			done <- callResult{out: v.Export()}
			return
		}

		// Promise：若已同步敲定则直接取结果；pending 则由后台轮询，让出 loop 处理异步回调
		if pm.State() != goja.PromiseStatePending {
			done <- settlePromise(pm)
			return
		}
		go p.awaitPromise(pm, done)
	})
	if !loopOK {
		// 循环已终止（插件卸载中）：立即失败，不再等满外层兜底超时
		return nil, fmt.Errorf("插件事件循环已终止，无法执行回调")
	}
	select {
	case res := <-done:
		if res.err != nil {
			return nil, res.err
		}
		return res.out, nil
	case <-time.After(callAwaitTimeout + 10*time.Second): // 外层兜底超时
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
// 反复短回 p.loop（RunOnLoop 会顺带清空微任务队列），让 async 函数内的 await/fetch/WebSocket
// 回调能在 loop 上真实继续执行并使 Promise 兑现；一旦敲定即把结果投回 done。
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
			// 循环已终止：Promise 永远不会在死循环上兑现，立即失败
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
//
// 注意：执行回调（callFn）时不得持有 p.mu，否则回调内的 signal.all 会去 Lock 同一把锁而自锁。
func (p *plugin) fireEvent(topic string, payload any) []Outcome {
	p.mu.Lock()
	loaded := p.loaded
	subs := append([]*eventSub(nil), p.events[topic]...)
	p.mu.Unlock()
	// 加载未完成/加载失败/已卸载的插件不参与派发（防止半初始化或僵尸循环消费事件）
	if !loaded {
		return nil
	}
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

// callExport 调用插件导出的函数（callFunction 目标）。
// 注意：取到 handler 后须先释放 p.mu 再 callFn，避免导出函数内 signal.all 自锁。
func (p *plugin) callExport(fnName string, args []any) (any, error) {
	p.mu.Lock()
	h, ok := p.exports[fnName]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s 包拒绝响应：未导出函数 %s", p.ID, fnName)
	}
	return p.callFn(h, args...)
}
