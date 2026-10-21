package lunarloop

import (
	module "CorePersonality/engine/persistence"
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"context"
	"fmt"

	"github.com/dop251/goja"
)

// memory.go 记忆库适配器 v3 — 分组集合 + 全局类型化检索
//
// 桥接 goja JS 运行时与自实现的记忆库（engine/persistence）。
// JS 侧通过 memoryInit/memoryAdd/memoryQuery/memoryAddImage 等调用，
// 由 create.go 中的 vm.Set 注册到全局作用域。
//
// v3 变更：
//   - 集合不再由调用方指定：入库时按内容与集合名向量的相关性自动归组
//   - 查询只指定返回类型（text / image / audio），跨全部集合检索
//   - memoryInit 不再接收集合名，启动时一次性加载全部集合
//   - 新增 memoryGet（按 ID）/ memoryList（逐条列举）与 memoryAddAudio

// memoryInit 初始化记忆库实例（加载全部集合、补齐默认分组与集合名向量）
// 模型配置与月华共用 agent.embedding_* / agent.multimodal_*
func (class *Runtime) memoryInit(call goja.FunctionCall) goja.Value {
	// 确保全局 MemoryDB 实例已初始化（幂等，若宿主已初始化则复用）
	module.InitMemoryDB(*GeneralConfig.MemoryDBDir)

	if err := module.MemoryInitInstance(); err != nil {
		LoggerGeneral.Error("LunarCore", "记忆库实例初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// memoryListCollections 列出记忆库全部分组集合名（管理/调试用途，不参与检索参数）
func (class *Runtime) memoryListCollections(call goja.FunctionCall) goja.Value {
	module.InitMemoryDB(*GeneralConfig.MemoryDBDir)
	if err := module.MemoryInitInstance(); err != nil {
		LoggerGeneral.Error("LunarCore", "记忆库实例初始化失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{module.MemoryListCollections(), nil})
}

// memoryAdd 添加文本消息（内容自动归入最相关的分组集合）
// 参数: role, content
func (class *Runtime) memoryAdd(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAdd 参数不足, 需 2 个: role, content")})
	}

	role, _ := call.Argument(0).Export().(string)
	content, _ := call.Argument(1).Export().(string)

	ctx := context.Background()
	id, err := module.MemoryAddMessage(ctx, role, content)
	if err != nil {
		LoggerGeneral.Error("LunarCore", "添加文本记忆失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{id, nil})
}

// memoryAddWithTags 添加文本消息，携带调用方提供的显式标签与来源标记
// 参数: role, content, tags(字符串数组), source(可空)
// 返回: [id, error]；tags 为空时按正文嵌入
func (class *Runtime) memoryAddWithTags(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("memoryAddWithTags 参数不足, 需 3 个: role, content, tags")})
	}

	role, _ := call.Argument(0).Export().(string)
	content, _ := call.Argument(1).Export().(string)

	// 解析 tags 字符串数组
	exportedTags := call.Argument(2).Export()
	rawTags, ok := exportedTags.([]any)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("memoryAddWithTags 第 3 个参数需为字符串数组 tags")})
	}
	tags := make([]string, 0, len(rawTags))
	for _, t := range rawTags {
		if s, ok := t.(string); ok && s != "" {
			tags = append(tags, s)
		}
	}

	source := ""
	if len(call.Arguments) > 3 {
		source, _ = call.Argument(3).Export().(string)
	}

	ctx := context.Background()
	id, err := module.MemoryAddMessageWithTags(ctx, role, content, tags, source)
	if err != nil {
		LoggerGeneral.Error("LunarCore", "添加文本记忆(显式标签)失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{id, nil})
}

// memoryQuery 跨全部集合检索指定类型的相关记忆
// 参数: queryText, mediaType(text/image/audio), topK, forceGlobal(可选)
//
// forceGlobal 缺省 false：走「集合名向量」快速检索（先查最相近集合，必要时回退全量）；
// 传 true 时扫描全部集合，用于表情包这类稀疏数据，确保不漏。
func (class *Runtime) memoryQuery(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryQuery 参数不足, 需 3 个: queryText, mediaType, topK")})
	}

	queryText, _ := call.Argument(0).Export().(string)
	mediaType, _ := call.Argument(1).Export().(string)
	topK := int(call.Argument(2).ToInteger())
	forceGlobal := false
	if len(call.Arguments) > 3 {
		forceGlobal = call.Argument(3).ToBoolean()
	}

	ctx := context.Background()
	results, err := module.MemoryQueryWithOptions(ctx, queryText, mediaType, topK, module.MemoryQueryOptions{
		ForceGlobal: forceGlobal,
	})
	if err != nil {
		LoggerGeneral.Error("LunarCore", "查询记忆失败（类型 %s）: %v", mediaType, err)
		return class.runtime.ToValue([]any{nil, err})
	}

	return class.runtime.ToValue([]any{memoryResultsToObjects(results), nil})
}

// memoryKeywordSearch 跨全部集合按关键词/正则检索指定类型的记忆（不调用嵌入服务）
// 参数: mediaType, terms(字符串数组), useRegex(bool), limit
func (class *Runtime) memoryKeywordSearch(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 4 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryKeywordSearch 参数不足, 需 4 个: mediaType, terms, useRegex, limit")})
	}

	mediaType, _ := call.Argument(0).Export().(string)
	rawTerms, ok := call.Argument(1).Export().([]any)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryKeywordSearch 第 2 个参数需为字符串数组 terms")})
	}
	terms := make([]string, 0, len(rawTerms))
	for _, item := range rawTerms {
		if text, ok := item.(string); ok && text != "" {
			terms = append(terms, text)
		}
	}
	useRegex := call.Argument(2).ToBoolean()
	limit := int(call.Argument(3).ToInteger())

	results, err := module.MemoryKeywordSearch(mediaType, terms, useRegex, limit)
	if err != nil {
		LoggerGeneral.Error("LunarCore", "记忆关键词检索失败（类型 %s）: %v", mediaType, err)
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{memoryResultsToObjects(results), nil})
}

// memoryGet 按 ID 返回一条记忆（跨全部集合查找）
// 参数: id
func (class *Runtime) memoryGet(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryGet 参数不足, 需 1 个: id")})
	}
	id, _ := call.Argument(0).Export().(string)

	result, err := module.MemoryGetEntry(id)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}
	objects := memoryResultsToObjects([]module.MemoryQueryResult{*result})
	if len(objects) == 0 {
		return class.runtime.ToValue([]any{nil, nil})
	}
	return class.runtime.ToValue([]any{objects[0], nil})
}

// memoryList 逐条列举记忆条目（跨全部集合）
// 参数: mediaType(空=全部类型), limit(<=0 返回全部), offset(可空)
func (class *Runtime) memoryList(call goja.FunctionCall) goja.Value {
	mediaType := ""
	if len(call.Arguments) > 0 {
		mediaType, _ = call.Argument(0).Export().(string)
	}
	limit := 0
	if len(call.Arguments) > 1 {
		limit = int(call.Argument(1).ToInteger())
	}
	offset := 0
	if len(call.Arguments) > 2 {
		offset = int(call.Argument(2).ToInteger())
	}

	entries, _ := module.MemoryListEntries(mediaType, offset, limit)
	return class.runtime.ToValue([]any{memoryResultsToObjects(entries), nil})
}

// memoryResultsToObjects 把记忆查询结果映射为 JS 可读对象
func memoryResultsToObjects(results []module.MemoryQueryResult) []map[string]any {
	objects := make([]map[string]any, 0, len(results))
	for _, r := range results {
		obj := map[string]any{
			"id":         r.ID,
			"type":       r.Type,
			"role":       r.Role,
			"collection": r.Collection,
			"similarity": r.Similarity,
		}
		if r.Source != "" {
			obj["source"] = r.Source
		}
		if r.Timestamp != 0 {
			obj["timestamp"] = r.Timestamp
		}
		if r.Content != "" {
			obj["content"] = r.Content
		}
		if r.URL != "" {
			obj["url"] = r.URL
		}
		if r.Base64 != "" {
			// 历史数据兼容：旧记忆仍内联 base64
			obj["base64"] = r.Base64
		}
		objects = append(objects, obj)
	}
	return objects
}

// memoryDelete 按 ID 删除记忆（跨全部集合查找）
// 参数: id
func (class *Runtime) memoryDelete(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryDelete 参数不足, 需 1 个: id")})
	}
	id, _ := call.Argument(0).Export().(string)

	ctx := context.Background()
	if err := module.MemoryDeleteMessage(ctx, id); err != nil {
		LoggerGeneral.Error("LunarCore", "删除记忆失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}
	return class.runtime.ToValue([]any{true, nil})
}

// memoryAddImage 添加图片记忆（LLM 自动生成标签并归组）
// 参数: base64Image, [orientation, custom]
func (class *Runtime) memoryAddImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAddImage 参数不足, 需 1 个: base64Image")})
	}

	base64Image, _ := call.Argument(0).Export().(string)

	// 可选参数：识别取向与自定义参考文本（不传时默认自动处理）
	orientation := ""
	custom := ""
	if len(call.Arguments) > 1 {
		orientation, _ = call.Argument(1).Export().(string)
	}
	if len(call.Arguments) > 2 {
		custom, _ = call.Argument(2).Export().(string)
	}

	ctx := context.Background()
	id, err := module.MemoryAddImage(ctx, base64Image, orientation, custom)
	if err != nil {
		LoggerGeneral.Error("LunarCore", "添加图片记忆失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{id, nil})
}

// memoryAddAudio 添加音频记忆（按描述文本归组）
// 参数: base64Audio, description
func (class *Runtime) memoryAddAudio(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAddAudio 参数不足, 需 1 个: base64Audio")})
	}
	base64Audio, _ := call.Argument(0).Export().(string)
	description := ""
	if len(call.Arguments) > 1 {
		description, _ = call.Argument(1).Export().(string)
	}

	ctx := context.Background()
	id, err := module.MemoryAddAudio(ctx, base64Audio, description)
	if err != nil {
		LoggerGeneral.Error("LunarCore", "添加音频记忆失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}
	return class.runtime.ToValue([]any{id, nil})
}
