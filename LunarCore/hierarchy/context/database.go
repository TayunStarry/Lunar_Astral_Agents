package context

import (
	"LunarCore/hierarchy/memory"
	"fmt"

	"github.com/dop251/goja"
)

// shareDatabase 适配TypeScript调用的数据库操作功能，处理请求并转换结果格式
// 返回值: [Object, error] 数据库操作结果和错误信息
func (class *Adapters) shareDatabase(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("参数不足")})
	}

	request, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("request必须是对象")})
	}

	// 构建数据库请求
	dbRequest := memory.DatabaseRequest{}

	if transaction, ok := request["transaction"].(bool); ok {
		dbRequest.Transaction = transaction
	}

	// 转换操作列表
	if operations, ok := request["operations"].([]interface{}); ok {
		dbRequest.Operations = make([]interface{}, len(operations))
		for i, op := range operations {
			if opMap, ok := op.(map[string]any); ok {
				dbRequest.Operations[i] = opMap
			}
		}
	}

	// 执行数据库操作
	result := memory.ExecuteDatabaseRequest(dbRequest)

	// 转换结果格式
	response := map[string]any{
		"success":       result.Success,
		"error":         result.Error,
		"results":       result.Results,
		"total_time_ms": result.TotalTime,
		"operations":    result.Operations,
	}

	return class.runtime.ToValue([]any{response, nil})
}
