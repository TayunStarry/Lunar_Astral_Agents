package crystal

import (
	StarLTP "LunarSubsystem/LunarHarness/engine/ltp9"
)

// HandleInbound 消费 /ws 入站消息：ltp9/* 调试信封交由 LTP9 引擎处理，其余旁路。
// 由装配层注册为集线器的入站订阅者。
func HandleInbound(data []byte) {
	StarLTP.HandleInbound(data)
}

// Close 关闭琉璃人格格持有的引擎资源（进程退出前调用）
func Close() {
	StarLTP.Close()
}
