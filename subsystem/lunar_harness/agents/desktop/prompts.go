//go:build windows

package desktop

// prompts.go 提示词加载：角色提示词统一放在 prompts/ 下，代码只保留角色与工具边界。
//
// 提示词是数据而不是代码：改语气、改输出格式不需要动 Go 逻辑；文件由 prompts
// 包 embed 进二进制，运行期按名称读取。文件缺失属于构建配置错误，直接 panic，
// 避免带着空提示词去调模型。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/prompts"
)

// mustPrompt 读取内嵌提示词；缺失或为空时立即失败（提示词是必需依赖）。
func mustPrompt(name string) string {
	data, err := prompts.Read(name)
	if err != nil {
		panic(fmt.Sprintf("加载桌面操纵提示词 %s 失败: %v", name, err))
	}
	return string(data)
}
