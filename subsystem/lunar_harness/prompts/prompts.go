// Package prompts 收纳月华（Lunar）与内部智能体的全部提示词，统一在此维护。
//
// 使用方包括：
//   - Go 原生月华思维链（lunar 包，经 prompts.Read 读取）；
//   - Go 侧内置智能体（agents/Vision 视觉理解、agents/DocumentReader 文献解析、
//     executors/ArchiveReader 归档解析、agents/SearchPlanner 搜索者-规划、
//     agents/SearchSummarizer 搜索者-摘要、chain/viewer 哔哩观影、agents/desktop 桌面操纵、
//     agents/DialogueBridging 通讯桥接、agents/MutualForum 互助论坛、
//     agents/file/ManipulateFiles 文件操纵、agents/file/ManipulateArchive 归档操纵、
//     agents/file/CodeExecution 代码执行、agents/file/ContentRollback 内容回滚、
//     agents/file/FileOperations 文件操作）。
//
// 提示词是数据而不是代码，集中到独立的 prompts/；运行期间提示词经 prompts.Read
// 以 "prompts/xxx.md" 的虚拟路径读取，调用方无需感知真实文件布局。
package prompts

import (
	"embed"
	"fmt"
	"strings"
)

// files 内嵌的全部提示词（角色提示词、任务模板、外观提示词）。
//
//go:embed *.md
var files embed.FS

// BasePersonaPlaceholder 各链提示词引用基础人设的占位符。
const BasePersonaPlaceholder = "{base-persona}"

// Read 读取内嵌提示词。
//
// name 为 prompts/ 下的文件名（如 "dialogueRole.md"），也接受带 "prompts/"
// 前缀的写法——TypeScript 侧的调用形态正是 `fileView('prompts/dialogueRole.md')`。
//
// 含 {base-persona} 时自动注入 dialogueRole.md 的基础人设段落，
// 让生活 / 创作 / 演绎等链共用同一份人格描述，避免出现第二套人设。
func Read(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("提示词名称为空")
	}
	base := strings.TrimPrefix(name, "prompts/")
	data, err := readEmbedded(base)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(data), BasePersonaPlaceholder) {
		data = []byte(strings.ReplaceAll(string(data), BasePersonaPlaceholder, BasePersona()))
	}
	return data, nil
}

// ReadOr 读取提示词；读取失败时返回 fallback，供智能体在缺文件时保持可用。
func ReadOr(name, fallback string) string {
	data, err := Read(name)
	if err != nil {
		return fallback
	}
	return string(data)
}

// BasePersona 返回人格对话提示词中的基础人设（「我是谁」段落）。
//
// 只提取跨场景通用的名字、性格、家人与厌恶，剥离对话框独有的当前位置，
// 供生活 / 创作 / 演绎提示词通过 {base-persona} 引用。
func BasePersona() string {
	data, err := readEmbedded("dialogueRole.md")
	if err != nil {
		return ""
	}
	lines := make([]string, 0)
	inSection := false
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if inSection {
				break
			}
			inSection = trimmed == "## 我是谁"
			continue
		}
		if !inSection {
			continue
		}
		// 当前位置属于对话场景，由对话运行时消息单独提供。
		if strings.Contains(line, "{current-address}") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// readEmbedded 读取内嵌提示词原文，不做占位符替换。
func readEmbedded(base string) ([]byte, error) {
	data, err := files.ReadFile(base)
	if err != nil {
		return nil, fmt.Errorf("读取内嵌提示词 %s 失败: %w", base, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("内嵌提示词 %s 为空", base)
	}
	return data, nil
}
