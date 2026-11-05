// Package contracts 内嵌运行时 API 契约文档（*API.d.ts）。
//
// 这些文档描述脚本沙箱可用的全局能力，由创作链的 read_dts 工具按需查阅；
// 它们是「运行时 API 契约」而不是提示词，因此从 prompts 包独立出来，
// 与 engine/runtime 的实现放在同一层，避免提示词包承载代码契约。
package contracts

// contracts.go 契约文档的内嵌、列举与读取。

import (
	"embed"
	"fmt"
	"strings"
)

// files 内嵌的全部运行时 API 契约（按能力拆分的 *API.d.ts）。
//
//go:embed *.d.ts
var files embed.FS

// ListDTS 列出全部内嵌运行时 API 契约（按文件名升序）。
func ListDTS() ([]DTSDoc, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("列出内嵌运行时契约失败: %w", err)
	}
	docs := make([]DTSDoc, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".d.ts") {
			continue
		}
		info, ierr := entry.Info()
		if ierr != nil {
			continue
		}
		docs = append(docs, DTSDoc{Name: entry.Name(), Size: info.Size()})
	}
	return docs, nil
}

// Read 读取一份内嵌运行时 API 契约。
//
// name 必须是精确裸文件名（如 "agentAPI.d.ts"）：read_dts 的既有语义只接受
// 清单中的文件名，传 "runtimeAPI/agentAPI.d.ts" 或 "prompts/xxx.d.ts" 都会被拒绝。
func Read(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("运行时契约名称为空")
	}
	if strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return nil, fmt.Errorf("运行时契约只接受精确裸文件名，不接受路径 %q", name)
	}
	data, err := files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("读取运行时契约 %s 失败: %w", name, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("运行时契约 %s 为空", name)
	}
	return data, nil
}
