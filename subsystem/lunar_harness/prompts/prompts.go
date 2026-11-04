// Package prompts 收纳月华（Lunar）与内部智能体的全部提示词，统一在此维护。
//
// 使用方包括：
//   - Go 原生月华思维链（lunar 包，经 prompts.Read 读取）；
//   - Go 侧内置智能体（agents/Vision 视觉理解、agents/DocumentReader 文献解析、
//     executors/ArchiveReader 归档解析、agents/ArchiveOps 归档操作、
//     agents/SearchPlanner 搜索者-规划、agents/SearchSummarizer 搜索者-摘要、
//     chain/viewer 哔哩观影、agents/desktop 桌面操纵、agents/DialogueBridging 通讯桥接、
//     agents/MutualForum 互助论坛、agents/FileEditor 文件编辑）。
//
// 提示词是数据而不是代码，集中到独立的 prompts/；运行期间提示词经 prompts.Read
// 以 "prompts/xxx.md" 的虚拟路径读取，运行时 API 契约则收纳在 prompts/runtimeAPI/
// 子目录（读取时仍可用裸文件名），调用方无需感知真实文件布局。
package prompts

import (
	"embed"
	"fmt"
	"strings"
)

// dtsDir 运行时 API 契约的存放目录（相对 prompts 包）。
const dtsDir = "runtimeAPI"

// files 内嵌的全部提示词（角色提示词、任务模板、外观提示词）与运行时 API 契约
// （runtimeAPI/*API.d.ts，按能力拆分，供创作 / 文件编辑智能体写脚本前按需查阅）。
//
//go:embed *.md runtimeAPI/*.d.ts
var files embed.FS

// ListDTS 列出全部内嵌运行时 API 契约（按文件名升序）。
func ListDTS() ([]DTSDoc, error) {
	entries, err := files.ReadDir(dtsDir)
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

// Read 读取内嵌提示词或运行时 API 契约。
//
// name 为 prompts/ 下的文件名（如 "dialogueRole.md"），也接受带 "prompts/"
// 前缀的写法——TypeScript 侧的调用形态正是 `fileView('prompts/dialogueRole.md')`。
// 运行时 API 契约（*.d.ts）统一落在 runtimeAPI/ 子目录：既可传裸文件名
// （如 "agentAPI.d.ts"），也可传 "runtimeAPI/agentAPI.d.ts"。
func Read(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("提示词名称为空")
	}
	base := strings.TrimPrefix(name, "prompts/")
	paths := []string{base}
	if strings.HasSuffix(base, ".d.ts") && !strings.HasPrefix(base, dtsDir+"/") {
		paths = append(paths, dtsDir+"/"+base)
	}
	var lastErr error
	for _, p := range paths {
		data, err := files.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("内嵌提示词 %s 为空", p)
		}
		return data, nil
	}
	return nil, fmt.Errorf("读取内嵌提示词 %s 失败: %w", base, lastErr)
}
