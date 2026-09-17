package engine

// 知识库存储：SQLite（local_data/database/knowledge.db，经共享子系统 FileManager/module 的
// 知识条目通道 KnowledgeLoadEntries / KnowledgeReplaceEntries，与琉璃 /knowledge/ 端点同库）。
// goja 全局签名保持不变（TypeScript 侧零改动）：
//   knowledgeLoad(table)           → [entries, error]  entries 为 [[key,text],...]
//   knowledgeSave(table, entries)  → [bool, error]     全表替换语义
// 不保留任何旧版本数据格式（JSON 文件等）的迁移策略。

import (
	"fmt"

	"LunarSubsystem/FileManager/module"

	"github.com/dop251/goja"
)

// knowledgeLoad 读取指定表的全部条目（格式 [[key,text],...]），返回值: [entries, error]
func (class *Runtime) knowledgeLoad(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("knowledgeLoad 参数不足，需 table 表名")})
	}
	table, _ := call.Argument(0).Export().(string)
	entries, err := module.KnowledgeLoadEntries(table)
	if err != nil {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("读取知识库表 %s 失败: %w", table, err)})
	}
	return class.runtime.ToValue([]any{entries, nil})
}

// knowledgeSave 以全表替换语义写入指定表条目数组，返回值: [boolean, error]
func (class *Runtime) knowledgeSave(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("knowledgeSave 参数不足，需 table 与 entries")})
	}

	table, _ := call.Argument(0).Export().(string)
	exported := call.Argument(1).Export()
	rawArr, ok := exported.([]any)
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("knowledgeSave 第 2 个参数需为条目数组")})
	}

	// 规整为 [key,value] 二元组，忽略畸形成员
	entries := make([][]string, 0, len(rawArr))
	for _, item := range rawArr {
		pair, ok := item.([]any)
		if !ok || len(pair) < 1 {
			continue
		}
		key, _ := pair[0].(string)
		val := ""
		if len(pair) >= 2 {
			if s, ok := pair[1].(string); ok {
				val = s
			}
		}
		entries = append(entries, []string{key, val})
	}

	if err := module.KnowledgeReplaceEntries(table, entries); err != nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("写入知识库表 %s 失败: %w", table, err)})
	}
	return class.runtime.ToValue([]any{true, nil})
}
