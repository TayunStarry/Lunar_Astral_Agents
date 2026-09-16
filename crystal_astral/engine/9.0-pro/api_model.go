package YaraLTP

// ==== LLM 模型 API：yara.model ====

import (
	"github.com/dop251/goja"
)

// bindModel 注入 yara.model。
func bindModel(vm *goja.Runtime, parent *goja.Object) {
	o := newObj(vm)

	// 从 params 提取 messages/tools/taskType/overrides
	objSetFn(o, "chat", func(call goja.FunctionCall) goja.Value {
		p := argMap(call, 0)
		res, err := modelChatFromParams(p)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"content": res})
	})
	objSetFn(o, "chatWithTask", func(call goja.FunctionCall) goja.Value {
		_ = argString(call, 0) // taskType：本实现统一走默认模型配置
		messages := rawMessages(call.Argument(1))
		res, err := chatComplete(messages, nil, "", "", "")
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"content": res})
	})
	objSetFn(o, "chatWithConfig", func(call goja.FunctionCall) goja.Value {
		p := argMap(call, 0)
		messages := mapGetAnyList(p, "messages")
		base := mapGetStr(p, "baseUrl")
		key := mapGetStr(p, "apiKey")
		model := mapGetStr(p, "model")
		res, err := chatComplete(messages, nil, base, key, model)
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"content": res})
	})
	objSetFn(o, "chatWithTools", func(call goja.FunctionCall) goja.Value {
		p := argMap(call, 0)
		messages := mapGetAnyList(p, "messages")
		tools := mapGetAnyList(p, "tools")
		res, err := chatComplete(toolsToMessage(messages), toolsToMessage(tools), "", "", "")
		if err != nil {
			return vm.ToValue(map[string]any{"error": err.Error()})
		}
		return vm.ToValue(map[string]any{"content": res})
	})
	objSetFn(o, "embed", func(call goja.FunctionCall) goja.Value {
		p := argMap(call, 0)
		var texts []string
		if t := mapGetStr(p, "text"); t != "" {
			texts = append(texts, t)
		}
		if arr, ok := p["texts"].([]any); ok {
			for _, v := range arr {
				texts = append(texts, toString(v))
			}
		}
		if len(texts) == 0 {
			return vm.ToValue(map[string]any{"error": "缺少 text 或 texts 参数"})
		}
		switch len(texts) {
		case 1:
			vec, err := embedText(texts[0])
			if err != nil {
				return vm.ToValue(map[string]any{"error": err.Error()})
			}
			return vm.ToValue(map[string]any{"embedding": vec})
		default:
			vecs, err := embedTexts(texts)
			if err != nil {
				return vm.ToValue(map[string]any{"error": err.Error()})
			}
			return vm.ToValue(map[string]any{"embeddings": vecs})
		}
	})
	objSetFn(o, "getConfig", func(call goja.FunctionCall) goja.Value {
		p := argMap(call, 0)
		return vm.ToValue(modelConfigSummary(mapGetStr(p, "taskType")))
	})
	objSetFn(o, "getAllConfigs", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(map[string]any{"agent": modelConfigSummary("agent")})
	})
	objSetFn(o, "getAvailableConfigs", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue([]any{modelConfigSummary("agent")})
	})
	objSetFn(o, "listTasks", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue([]string{"agent"})
	})
	objSetFn(o, "getAvailableModels", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue([]string{modelChatName()})
	})

	parent.Set("model", o)
}

// rawMessages 把 JS 参数导出为 []map。
func rawMessages(v goja.Value) []map[string]any {
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	if arr, ok := v.Export().([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			if m, is := e.(map[string]any); is {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// mapGetAnyList 读取 map 的数组字段为 []map。
func mapGetAnyList(m map[string]any, key string) []map[string]any {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok {
		return nil
	}
	if arr, is := raw.([]any); is {
		out := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			if mm, ok := e.(map[string]any); ok {
				out = append(out, mm)
			}
		}
		return out
	}
	return nil
}

// toolsToMessage 把 []map 原样透传（tools 与 messages 都已是 map 结构）。
func toolsToMessage(x []map[string]any) []map[string]any { return x }

// modelConfigSummary 输出当前模型配置概要。
func modelConfigSummary(taskType string) map[string]any {
	chatCfg()
	return map[string]any{
		"task_type":     taskType,
		"model":         chatModel,
		"base_url":      chatURL,
		"embedding":     embedModel,
		"embedding_url": embedURL,
	}
}