package FileEditor

// repair.go 向拯救者注册文件编辑的修复能力。
//
// 文件编辑按次执行（每次从系统提示词 + 本次指令重新开始），没有跨轮上下文，
// 因此只提供 reset：清掉运行期标记，让下一次调用从干净状态开始。

import (
	"LunarSubsystem/LunarHarness/protocol/agentrepair"
)

func init() {
	agentrepair.RegisterResettable("file_editor", "文件编辑",
		"文件编辑按次执行、不保留对话上下文；reset 清理运行期状态",
		resetEditor,
		func(int) (any, error) { return exportContext(false) })
}

// resetEditor 文件编辑没有持久上下文，重置是安全且幂等的。
func resetEditor() string {
	return "文件编辑按次执行，没有可清理的对话上下文"
}
