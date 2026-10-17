// Package prompts 收纳月华（Lunar）智能体的全部提示词，统一在此维护。
//
// 提示词是数据而不是代码，此前散落在 hierarchy/assets/prompts/ 这类"前端资源"
// 目录下，与 CSS/JS 混在一起。现在集中到独立的 prompts/，运行期由
// engine/LunarLoop 的 fileView 适配器以 "prompts/xxx.md" 的虚拟路径读取，
// TypeScript 侧与已打包的 agentSystem.js 都无需感知真实文件布局。
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
