// Package prompts 收纳月华（Lunar）与内部智能体的全部提示词，统一在此维护。
//
// 使用方包括：
//   - Go 原生月华对话链（lunar 包，经 prompts.Read 读取）；
//   - Go 侧内置智能体（agents/Descriptor 描述者、agents/WebSearch 网络搜索、agents/BilibiliViewer 哔哩观影、
//     agents/desktop 桌面操纵、agents/DialogueBridging 通讯桥接、agents/MutualForum 互助论坛）。
//
// 提示词是数据而不是代码，集中到独立的 prompts/，运行期经 prompts.Read 以
// "prompts/xxx.md" 的虚拟路径读取，调用方无需感知真实文件布局。
package prompts

import (
	"embed"
	"fmt"
	"strings"
)

// files 内嵌的全部提示词（角色提示词、任务模板、外观提示词）
//
//go:embed *.md
var files embed.FS

// Read 读取内嵌提示词。
//
// name 为 prompts/ 下的文件名（如 "dialogueRole.md"），也接受带 "prompts/"
// 前缀的写法——TypeScript 侧的调用形态正是 `fileView('prompts/dialogueRole.md')`。
func Read(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("提示词名称为空")
	}
	base := strings.TrimPrefix(name, "prompts/")
	data, err := files.ReadFile(base)
	if err != nil {
		return nil, fmt.Errorf("读取内嵌提示词 %s 失败: %w", base, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("内嵌提示词 %s 为空", base)
	}
	return data, nil
}
