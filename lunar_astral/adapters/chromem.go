package adapters

import (
	"context"
	"fmt"
	"logger"
	"storage/module"

	"github.com/dop251/goja"
)

// chromemInit 初始化 chromem-go
func (class *Runtime) chromemInit(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("chromemInit 参数不足")})
	}

	baseURL, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("baseURL 必须是字符串")})
	}

	apiKey, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("apiKey 必须是字符串")})
	}

	modelName, ok := call.Argument(2).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("modelName 必须是字符串")})
	}

	err := module.Init(baseURL, apiKey, modelName)
	if err != nil {
		logger.Error("LunarCore", "chromem 初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// chromemAdd 添加消息到 chromem-go
func (class *Runtime) chromemAdd(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("chromemAdd 参数不足")})
	}

	role, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("role 必须是字符串")})
	}

	content, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("content 必须是字符串")})
	}

	ctx := context.Background()
	err := module.AddMessage(ctx, role, content)
	if err != nil {
		logger.Error("LunarCore", "chromem 添加消息失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// chromemQuery 查询 chromem-go 相关消息
func (class *Runtime) chromemQuery(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("chromemQuery 参数不足")})
	}

	queryText, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("queryText 必须是字符串")})
	}

	topK := int(call.Argument(1).ToInteger())

	ctx := context.Background()
	messages, err := module.QueryMessagesWithContent(ctx, queryText, topK)
	if err != nil {
		logger.Error("LunarCore", "chromem 查询消息失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}

	resultObjs := make([]map[string]string, 0, len(messages))
	for _, msg := range messages {
		resultObjs = append(resultObjs, map[string]string{
			"id":      msg["id"],
			"role":    msg["role"],
			"content": msg["content"],
		})
	}

	return class.runtime.ToValue([]any{resultObjs, nil})
}

// chromemDelete 从 chromem-go 删除指定消息
func (class *Runtime) chromemDelete(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("chromemDelete 参数不足")})
	}

	id, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("id 必须是字符串")})
	}

	ctx := context.Background()
	err := module.DeleteMessage(ctx, id)
	if err != nil {
		logger.Error("LunarCore", "chromem 删除消息失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}
