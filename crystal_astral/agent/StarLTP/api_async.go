package StarLTP

// ==== engine.async 异步子任务（常驻基础能力） ====
// async.run(taskFn, options) 在后台 goroutine 中调度任务，返回 taskId 供进度/状态查询；
// taskFn 收到 (task) 参数，task.id 即 taskId（可配合 reportProgress 上报进度）。
// 注意：taskFn 仍在插件事件循环线程内执行（复用 callFn），因此与同步模型一致——
// 任务运行期间会占用事件循环，但调用方可以先把任务「交给后台」再继续自己的主流程。

import (
	"fmt"
	"time"

	"github.com/dop251/goja"
)

// bindAsync engine.async.run/reportProgress/getStatus/list（常驻注入）。
func bindAsync(vm *goja.Runtime, p *plugin) *goja.Object {
	a := vm.NewObject()
	a.Set("run", func(taskFn goja.Value, options map[string]any) (any, error) {
		if taskFn == nil || !isFunc(taskFn) {
			return rwResult{Success: false, Error: "taskFn 需为函数"}, nil
		}
		var timeoutMs float64
		if o, ok := options["timeout"].(float64); ok {
			timeoutMs = o
		}
		var data any
		if d, ok := options["data"]; ok {
			data = d
		}
		return asyncRun(p, taskFn, data, timeoutMs)
	})
	a.Set("reportProgress", func(id int, progress any) map[string]any { return asyncReportProgress(p, id, progress) })
	a.Set("getStatus", func(id int) map[string]any { return asyncGetStatus(p, id) })
	a.Set("list", func() []any { return asyncList(p) })
	return a
}

// asyncRun 登记并启动一个异步子任务，返回 taskId。
func asyncRun(p *plugin, fn jsFunc, data any, timeoutMs float64) (any, error) {
	p.asyncMu.Lock()
	p.asyncSeq++
	id := p.asyncSeq
	p.asyncTasks[id] = &asyncTask{id: id, status: "running", data: data, fn: fn}
	p.asyncMu.Unlock()

	go func() {
		taskObj := map[string]any{"id": id, "data": data}
		_, err := p.callFn(fn, taskObj)
		p.asyncMu.Lock()
		ts, ok := p.asyncTasks[id]
		if ok {
			if err != nil {
				ts.status = "error"
			} else if ts.status == "running" {
				ts.status = "done"
			}
		}
		p.asyncMu.Unlock()
	}()

	// 超时看门狗（仅标记状态，不强制中断阻塞中的任务）
	if timeoutMs > 0 {
		go func() {
			time.Sleep(time.Duration(timeoutMs) * time.Millisecond)
			p.asyncMu.Lock()
			if ts, ok := p.asyncTasks[id]; ok && ts.status == "running" {
				ts.status = "timeout"
			}
			p.asyncMu.Unlock()
		}()
	}
	return rwResult{Success: true, Text: fmt.Sprint(id)}, nil
}

// asyncReportProgress 记录任务进度（可由任务自身或外部调用）。
func asyncReportProgress(p *plugin, id int, progress any) map[string]any {
	p.asyncMu.Lock()
	defer p.asyncMu.Unlock()
	ts, ok := p.asyncTasks[id]
	if !ok {
		return map[string]any{"success": false, "error": "任务不存在"}
	}
	ts.progress = progress
	return map[string]any{"success": true}
}

// asyncGetStatus 查询单个任务状态。
func asyncGetStatus(p *plugin, id int) map[string]any {
	p.asyncMu.Lock()
	defer p.asyncMu.Unlock()
	ts, ok := p.asyncTasks[id]
	if !ok {
		return map[string]any{"success": false, "error": "任务不存在"}
	}
	return map[string]any{"success": true, "taskId": ts.id, "status": ts.status, "progress": ts.progress}
}

// asyncList 枚举当前全部任务状态。
func asyncList(p *plugin) []any {
	p.asyncMu.Lock()
	defer p.asyncMu.Unlock()
	out := make([]any, 0, len(p.asyncTasks))
	for _, ts := range p.asyncTasks {
		out = append(out, map[string]any{"taskId": ts.id, "status": ts.status, "progress": ts.progress})
	}
	return out
}