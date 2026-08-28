package adapters

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
)

// knowledgeDir 知识库 JSON 数据目录（位于 local_data/database 下，与记忆库 memory 同级）
const knowledgeDir = "local_data/database/knowledge"

// knowledgeFilePath 拼装某个表对应的 JSON 文件完整路径
// 一个表对应一个 JSON 文件，文件名即表名，数据格式为 [key,text][]
func knowledgeFilePath(table string) (string, error) {
	// 仅允许简单表名，防止路径穿越
	if table == "" || strings.ContainsAny(table, `/\`) {
		return "", fmt.Errorf("无效的知识库表名: %q", table)
	}
	return filepath.Join(knowledgeDir, table+".json"), nil
}

// knowledgeLoad 读取指定表的 JSON（格式 [[key,text],...]）并返回条目数组
// 返回值: [entries, error]；文件不存在或为空时返回空数组
func (class *Runtime) knowledgeLoad(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return class.runtime.ToValue([]any{nil, fmt.Errorf("knowledgeLoad 参数不足，需 table 表名")})
	}

	table, _ := call.Argument(0).Export().(string)
	path, err := knowledgeFilePath(table)
	if err != nil {
		return class.runtime.ToValue([]any{nil, err})
	}

	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return class.runtime.ToValue([]any{[][]string{}, nil})
		}
		return class.runtime.ToValue([]any{nil, fmt.Errorf("读取知识库 %s 失败: %w", path, readErr)})
	}

	// 数据格式 [key,text][]：每项为 [key, value] 二元组
	entries := make([][]string, 0)
	if text := strings.TrimSpace(string(raw)); text != "" {
		if jsonErr := json.Unmarshal([]byte(text), &entries); jsonErr != nil {
			return class.runtime.ToValue([]any{nil, fmt.Errorf("解析知识库 %s 失败: %w", path, jsonErr)})
		}
	}
	return class.runtime.ToValue([]any{entries, nil})
}

// knowledgeSave 将指定表的 [key,text][] 条目数组写回 JSON 文件（自动建目录，覆写模式）
// 返回值: [boolean, error]
func (class *Runtime) knowledgeSave(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		return class.runtime.ToValue([]any{false, fmt.Errorf("knowledgeSave 参数不足，需 table 与 entries")})
	}

	table, _ := call.Argument(0).Export().(string)
	path, err := knowledgeFilePath(table)
	if err != nil {
		return class.runtime.ToValue([]any{false, err})
	}

	exported := call.Argument(1).Export()
	rawArr, ok := exported.([]interface{})
	if !ok {
		return class.runtime.ToValue([]any{false, fmt.Errorf("knowledgeSave 第 2 个参数需为条目数组")})
	}

	// 规整为 [key,value] 二元组，忽略畸形成员
	entries := make([][]string, 0, len(rawArr))
	for _, item := range rawArr {
		pair, ok := item.([]interface{})
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

	data, _ := json.MarshalIndent(entries, "", "\t")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("创建知识库目录失败: %w", err)})
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return class.runtime.ToValue([]any{false, fmt.Errorf("写入知识库 %s 失败: %w", path, err)})
	}
	return class.runtime.ToValue([]any{true, nil})
}