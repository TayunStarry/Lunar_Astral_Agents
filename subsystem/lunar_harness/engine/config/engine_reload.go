package config

import "flag"

// ==== 引擎自愈（月华 goja 运行时的重载策略）====
//
// 为什么需要：goja 运行时是一整块内存状态（角色实例、上下文、思考中标记、
// setInterval 计时器）。它一旦因为外部原因（实测：llama-server 因内部断言崩溃、
// 连接被拒）走进死路，**进程内没有任何东西能把它救回来**，只能重启整个程序。
// 而重启程序会连带重启前端窗口与全部子系统——代价远大于丢一段对话上下文。

var (
	// EngineAutoReload 应答连续失败达阈值时，是否自动重载月华引擎（goja lunar_loop）
	EngineAutoReload = flag.Bool("engine-auto-reload", true, "月华引擎持续失败时自动重载(goja 运行时)")
	// EngineAutoReloadFailures 连续失败多少次触发自动重载（1 会让一次网络抖动就重置上下文）
	EngineAutoReloadFailures = flag.Int("engine-auto-reload-failures", 2, "触发自动重载的连续应答失败次数")
)
