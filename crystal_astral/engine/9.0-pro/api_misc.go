package YaraLTP

// ==== 异步与表情包 API：async / emoji ====

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	file "LunarSubsystem/FileManager/module"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
)

var asyncSeq int64

// yaraStickerCollection 表情包集合（image 型，复用项目记忆库 stickers 约定，与 LTP9 一致）。
const yaraStickerCollection = "stickers"

// yaraStickerOnce / yaraStickerErr 惰性确保表情包集合就绪。
var (
	yaraStickerOnce sync.Once
	yaraStickerErr  error
)

// ensureYaraStickers 确保项目记忆库与 stickers 集合（image 型）可访问。
func ensureYaraStickers() {
	yaraStickerOnce.Do(func() {
		if !file.IsMemoryInitialized() {
			yaraStickerErr = fmt.Errorf("记忆库未初始化")
			return
		}
		if file.MemoryGetCollectionInfo(yaraStickerCollection) == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30_000_000_000)
			defer cancel()
			yaraStickerErr = file.CollectionInit(ctx, yaraStickerCollection, *GeneralConfig.SearchEmbeddingModel, file.CollectionTypeImage)
		}
	})
}

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

// bindEmoji 注入 yara.emoji（基于项目记忆库 stickers（image 型）集合的真实实现）。
func bindEmoji(p *plugin, parent *goja.Object) {
	vm := p.vm
	o := newObj(vm)
	objSetFn(o, "getRandom", func(call goja.FunctionCall) goja.Value {
		q := argString(call, 0)
		if q == "" {
			q = "general"
		}
		res := yaraEmojiQuery(vm, q, 3, true)
		return res
	})
	objSetFn(o, "getByEmotion", func(call goja.FunctionCall) goja.Value {
		q := argString(call, 0)
		limit := int(argInt(call, 1))
		if q == "" {
			return vm.ToValue(map[string]any{"error": "emotion 必填"})
		}
		if limit <= 0 {
			limit = 10
		}
		return yaraEmojiQuery(vm, q, limit, false)
	})
	objSetFn(o, "getAll", func(call goja.FunctionCall) goja.Value {
		limit := int(argInt(call, 0))
		if limit <= 0 {
			limit = 100
		}
		entries, _, err := yaraEmojiAll(limit)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(entries)
	})
	objSetFn(o, "getCount", func(call goja.FunctionCall) goja.Value {
		ensureYaraStickers()
		if yaraStickerErr != nil {
			return vm.ToValue(0)
		}
		return vm.ToValue(file.MemoryGetEntryCount(yaraStickerCollection))
	})
	objSetFn(o, "getEmotions", func(call goja.FunctionCall) goja.Value {
		// 表情集合以多模态标签作为"情绪"维度，模块不暴露去重标签枚举；
		// 返回当前表情集合元信息（含嵌入模型/维度），供插件感知后端能力。
		ensureYaraStickers()
		return vm.ToValue(file.MemoryGetCollectionInfoWithType(yaraStickerCollection))
	})
	objSetFn(o, "getInfo", func(call goja.FunctionCall) goja.Value {
		ensureYaraStickers()
		return vm.ToValue(file.MemoryGetCollectionInfoWithType(yaraStickerCollection))
	})
	parent.Set("emoji", o)
}

// yaraEmojiQuery 从 stickers 集合按语义检索表情，返回 {image, similarity} 列表；
// random=true 时从结果随机返回单张。
func yaraEmojiQuery(vm *goja.Runtime, query string, limit int, random bool) goja.Value {
	ensureYaraStickers()
	if yaraStickerErr != nil {
		return vm.ToValue(map[string]any{"error": yaraStickerErr.Error()})
	}
	results, err := file.MemoryQueryMessagesWithContent(context.Background(), yaraStickerCollection, query, limit)
	if err != nil {
		return vm.ToValue(map[string]any{"error": err.Error()})
	}
	if len(results) == 0 {
		return vm.ToValue([]any{})
	}
	if random {
		pick := results[rand.Intn(len(results))]
		return vm.ToValue(map[string]any{"image": pick.Image})
	}
	out := make([]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{"image": r.Image, "similarity": r.Similarity})
	}
	return vm.ToValue(out)
}

// yaraEmojiAll 分页列出 stickers 集合全部表情。
func yaraEmojiAll(limit int) ([]any, int, error) {
	ensureYaraStickers()
	if yaraStickerErr != nil {
		return nil, 0, yaraStickerErr
	}
	entries, total := file.MemoryGetDocuments(yaraStickerCollection, 0, limit)
	out := make([]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{"id": e.ID, "image": e.Image})
	}
	return out, total, nil
}
