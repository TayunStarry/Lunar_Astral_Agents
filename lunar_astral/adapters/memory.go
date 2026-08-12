package adapters

import (
	"LunarSubsystem/FileManager/module"
	config "LunarSubsystem/GeneralConfig"
	logger "LunarSubsystem/LoggerGeneral"
	"context"
	"fmt"

	"github.com/dop251/goja"
)

// memory.go 记忆库适配器 v2 — 标签向量中介检索架构
//
// 桥接 goja JS 运行时与自实现的记忆库（subsystem/storage/module）。
// JS 侧通过 memoryInit/memoryAdd/memoryQuery/memoryDelete/memoryAddImage 调用，
// 由 create.go 中的 vm.Set 注册到全局作用域。
//
// v2 变更：
//   - memoryInit 新增 LLM 配置参数和 collectionType
//   - memoryInitImage 已移除（统一到 memoryInit）
//   - memoryQueryImage 已移除（统一到 memoryQuery）
//   - memoryAddImage 简化为 2 参数（LLM 自动生成标签）

// memoryInit 初始化记忆库实例并创建指定集合
// 模型配置全部从 lunar_config.json 的 memory 配置组读取
func (class *Runtime) memoryInit(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryInit 参数不足, 需 2 个: collectionName, collectionType")})
	}

	collectionName, _ := call.Argument(0).Export().(string)
	collectionType, _ := call.Argument(1).Export().(string)

	if collectionType == "" {
		collectionType = module.CollectionTypeText
	}

	// 确保全局 MemoryDB 实例已初始化（幂等，若 crystal_astral 已初始化则复用）
	module.InitMemoryDB("local_data/database/memory")

	// 第一步：实例初始化（嵌入服务 + LLM 标签生成服务，模型配置从 config 模块读取）
	if err := module.MemoryInitInstance(); err != nil {
		logger.Error("LunarCore", "记忆库实例初始化失败: %v", err)
		return class.runtime.ToValue([]any{false, err})
	}

	// 第二步：创建/打开集合（嵌入模型名从 memory.embedding_model 配置读取）
	ctx := context.Background()
	modelName := *config.MemoryEmbeddingModel
	if err := module.CollectionInit(ctx, collectionName, modelName, collectionType); err != nil {
		logger.Error("LunarCore", "集合 [%s] 创建失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// memoryAdd 添加消息到指定集合（同步阻塞，等待 LLM 标签生成完成）
// 参数: collectionName, role, content
func (class *Runtime) memoryAdd(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAdd 参数不足, 需 3 个: collectionName, role, content")})
	}

	collectionName, _ := call.Argument(0).Export().(string)
	role, _ := call.Argument(1).Export().(string)
	content, _ := call.Argument(2).Export().(string)

	ctx := context.Background()
	id, err := module.MemoryAddMessage(ctx, collectionName, role, content)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 添加消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{id, nil})
}

// memoryQuery 查询指定集合的相关消息（text 和 image 统一）
// 参数: collectionName, queryText, topK
func (class *Runtime) memoryQuery(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 3 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryQuery 参数不足, 需 3 个: collectionName, queryText, topK")})
	}

	collectionName, _ := call.Argument(0).Export().(string)
	queryText, _ := call.Argument(1).Export().(string)
	topK := int(call.Argument(2).ToInteger())

	ctx := context.Background()
	results, err := module.MemoryQueryMessagesWithContent(ctx, collectionName, queryText, topK)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 查询消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{nil, err})
	}

	resultObjs := make([]map[string]any, 0, len(results))
	for _, r := range results {
		obj := map[string]any{
			"id":         r.ID,
			"role":       r.Role,
			"similarity": r.Similarity,
		}
		if r.Image != "" {
			obj["image"] = r.Image
		} else {
			obj["content"] = r.Content
		}
		resultObjs = append(resultObjs, obj)
	}

	return class.runtime.ToValue([]any{resultObjs, nil})
}

// memoryDelete 从指定集合删除消息
// 参数: collectionName, id
func (class *Runtime) memoryDelete(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryDelete 参数不足, 需 2 个: collectionName, id")})
	}

	collectionName, _ := call.Argument(0).Export().(string)
	id, _ := call.Argument(1).Export().(string)

	ctx := context.Background()
	err := module.MemoryDeleteMessage(ctx, collectionName, id)
	if err != nil {
		logger.Error("LunarCore", "集合 [%s] 删除消息失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{false, err})
	}

	return class.runtime.ToValue([]any{true, nil})
}

// memoryAddImage 向 image 类型集合添加图片文档（LLM 自动生成标签）
// 参数: collectionName, base64Image
func (class *Runtime) memoryAddImage(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("memoryAddImage 参数不足, 需 2 个: collectionName, base64Image")})
	}

	collectionName, _ := call.Argument(0).Export().(string)
	base64Image, _ := call.Argument(1).Export().(string)

	ctx := context.Background()
	id, err := module.MemoryAddImage(ctx, collectionName, base64Image)
	if err != nil {
		logger.Error("LunarCore", "图片集合 [%s] 添加图片失败: %v", collectionName, err)
		return class.runtime.ToValue([]any{nil, err})
	}

	return class.runtime.ToValue([]any{id, nil})
}
