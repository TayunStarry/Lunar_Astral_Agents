package DocumentReader

// repair.go 向拯救者注册文献解析的修复能力。
//
// 文献解析按次执行（每次独立读文件、按需分批），不保留跨轮上下文；
// 失败通常来自文件本身，reset 只清理运行期标记。

import (
	"LunarSubsystem/LunarHarness/engine/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("document_reader", "文献解析",
		"文献解析按次执行、不保留对话上下文；reset 清理运行期状态",
		resetReader)
}

// resetReader 文献解析没有持久上下文，重置是安全且幂等的。
func resetReader() string {
	return "文献解析按次执行，没有可清理的对话上下文"
}
