package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"logger"
	"lunar_astral/model/chromem"

	"github.com/dop251/goja"
)

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

	err := chromem.Init(baseURL, apiKey, modelName)
	if err != nil {
		logger.Error("LunarCore", "chromem 初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

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
	err := chromem.AddMessage(ctx, role, content)
	if err != nil {
		logger.Error("LunarCore", "chromem 添加消息失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

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
	results, err := chromem.QueryMessages(ctx, queryText, topK)
	if err != nil {
		logger.Error("LunarCore", "chromem 查询消息失败: %v", err)
		return class.runtime.ToValue([]any{nil, err})
	}

	jsonResults, err := json.Marshal(results)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("序列化查询结果失败: %v", err)})
	}

	var parsedResults []any
	if err := json.Unmarshal(jsonResults, &parsedResults); err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("反序列化查询结果失败: %v", err)})
	}

	return class.runtime.ToValue([]any{parsedResults, nil})
}
