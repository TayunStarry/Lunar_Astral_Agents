package adapters

import (
	"context"
	"fmt"
	"logger"
	"storage/module"

	"github.com/dop251/goja"
)

// memory.go 记忆库适配器
//
// 桥接 goja JS 运行时与自实现的记忆库（subsystem/storage/module）。
// JS 侧通过 memoryInit/memoryAdd/memoryQuery/memoryDelete 调用，
// 由 create.go 中的 vm.Set 注册到全局作用域。

// memoryInit 初始化记忆库实例并创建指定集合
// 参数: baseURL, apiKey, modelName, collectionName
func (class *Runtime) memoryInit(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 4 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryInit 参数不足, 需 4 个: baseURL, apiKey, modelName, collectionName")})
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

	collectionName, ok := call.Argument(3).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("collectionName 必须是字符串")})
	}

	// 第一步：实例初始化（配置嵌入服务连接）
	if err := module.MemoryInitInstance(baseURL, apiKey); err != nil {
		logger.Error("LunarCore", "记忆库实例初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	// 第二步：创建/打开集合（探针定维度，锁定模型）
	ctx := context.Background()
	if err := module.CollectionInit(ctx, collectionName, modelName); err != nil {
		logger.Error("LunarCore", "集合 [%s] 创建失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// memoryAdd 添加消息到指定集合
// 参数: collectionName, role, content
func (class *Runtime) memoryAdd(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAdd 参数不足, 需 3 个: collectionName, role, content")})
	}

	collectionName, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("collectionName 必须是字符串")})
	}

	role, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("role 必须是字符串")})
	}

	content, ok := call.Argument(2).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("content 必须是字符串")})
	}

	ctx := context.Background()
	err := module.AddMessage(ctx, collectionName, role, content)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 添加消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// memoryQuery 查询指定集合的相关消息
// 参数: collectionName, queryText, topK
func (class *Runtime) memoryQuery(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryQuery 参数不足, 需 3 个: collectionName, queryText, topK")})
	}

	collectionName, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("collectionName 必须是字符串")})
	}

	queryText, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("queryText 必须是字符串")})
	}

	topK := int(call.Argument(2).ToInteger())

	ctx := context.Background()
	messages, err := module.QueryMessagesWithContent(ctx, collectionName, queryText, topK)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 查询消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{nil, err})
	}

	// 已按相似度降序返回结果
	resultObjs := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		resultObjs = append(resultObjs, map[string]any{
			"id":         msg.ID,
			"role":       msg.Role,
			"content":    msg.Content,
			"similarity": msg.Similarity,
		})
	}

	return class.runtime.ToValue([]any{resultObjs, nil})
}

// memoryDelete 从指定集合删除消息
// 参数: collectionName, id
func (class *Runtime) memoryDelete(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryDelete 参数不足, 需 2 个: collectionName, id")})
	}

	collectionName, ok := call.Argument(0).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("collectionName 必须是字符串")})
	}

	id, ok := call.Argument(1).Export().(string)
	if !ok {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("id 必须是字符串")})
	}

	ctx := context.Background()
	err := module.DeleteMessage(ctx, collectionName, id)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 删除消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}
