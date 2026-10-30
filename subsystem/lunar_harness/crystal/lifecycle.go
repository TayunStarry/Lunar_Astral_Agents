package crystal

// lifecycle.go 琉璃人格格持有的运行时资源收尾。

import (
	runtime "LunarSubsystem/LunarHarness/engine/runtime"
)

// Close 关闭月华私有 js 运行时（进程退出前调用）。
func Close() {
	runtime.Close()
}
