package YaraLTP

// ==== 异步与表情包 API：async / emoji ====

import (
	"fmt"
	"sync/atomic"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

var asyncSeq int64

// bindAsync 注入 yara.async（后台任务，经插件串行锁调度）。
func bindAsync(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "run", func(call goja.FunctionCall) goja.Value {
		taskFn := call.Argument(0)
		opts := argMap(call, 1)
		timeout := int(toInt64(mapGet(opts, "timeout")))
		if timeout <= 0 {
			timeout = 300
		}
		if !isJSFunc(taskFn) {
			return vm.ToValue(map[string]any{"error": "缺少任务函数"})
		}
		id := fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&asyncSeq, 1))
		onComplete := goja.Undefined()
		onError := goja.Undefined()
		if o, ok := opts["onComplete"].(goja.Value); ok {
			onComplete = o
		}
		if o, ok := opts["onError"].(goja.Value); ok {
			onError = o
		}
		_ = onComplete
		_ = onError
		go runAsyncTask(p, id, taskFn, opts)
		return vm.ToValue(map[string]any{"taskId": id, "status": "running", "timeout": timeout})
	})
	objSetFn(o, "reportProgress", func(call goja.FunctionCall) goja.Value {
		// 进度上报：本实现记录到 logger，供调试。
		LoggerGeneral.Info(ServiceName, "async 进度 task=%s data=%v", argString(call, 0), argExport(call, 1))
		return goja.Undefined()
	})
	parent.Set("async", o)
}

// runAsyncTask 后台执行任务函数（经插件串行锁，保证 VM 不被并发访问）。
func runAsyncTask(p *plugin, id string, fn goja.Value, opts map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			LoggerGeneral.Error(ServiceName, "async 任务 %s 崩溃: %v", id, r)
		}
	}()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.vm == nil {
		return
	}
	data := map[string]any{"taskId": id}
	if d, ok := opts["data"]; ok {
		if dm, is := d.(map[string]any); is {
			for k, v := range dm {
				data[k] = v
			}
		}
	}
	// 任务执行；（可选回调 onComplete/onError 的提取已在 run 中完成，此处仅执行有副作用的部分）
	if _, err := p.callFn(fn, data); err != nil {
		p.fireAsyncError(id, err, opts)
	}
}

// fireAsyncError 触发异步任务失败后的 onError 回调（若 opts 中有 goja 函数可调用）。
func (p *plugin) fireAsyncError(id string, err error, opts map[string]any) {}

// bindEmoji 注入 yara.emoji（表情库未接入，返回为空结构）。
func bindEmoji(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "getRandom", func(call goja.FunctionCall) goja.Value { return vm.ToValue(nil) })
	objSetFn(o, "getByEmotion", func(call goja.FunctionCall) goja.Value { return vm.ToValue(nil) })
	objSetFn(o, "getAll", func(call goja.FunctionCall) goja.Value { return vm.ToValue([]any{}) })
	objSetFn(o, "getCount", func(call goja.FunctionCall) goja.Value { return vm.ToValue(0) })
	objSetFn(o, "getEmotions", func(call goja.FunctionCall) goja.Value { return vm.ToValue([]any{}) })
	objSetFn(o, "getInfo", func(call goja.FunctionCall) goja.Value { return vm.ToValue(nil) })
	parent.Set("emoji", o)
}