package LunarPersonality

// lifecycle.go 月华人格局持有的运行时资源收尾。

import (
	runtime "LunarSubsystem/LunarHarness/engine/runtime"
)

// Close 关闭月华私有 js 运行时（进程退出前调用）。
// 该运行时由月华的创作 / 文件编辑链路使用，因此收尾归月华人格局而非琉璃。
func Close() {
	runtime.Close()
}
