package runtime

// ==== 常量与变量集中区 ====

import (
	"time"
)

// ServiceName 模块标识（日志分类据此归入「生活」）。
const ServiceName = "Runtime"

// AgentSearchFunc 进程内网络搜索实现，由装配层（app）注入。
// engine 层不得反向 import agents，故以函数钩子解耦；未注入时 agent.search 返回「能力未装配」。
var AgentSearchFunc func(instruction string) (string, error)

// nowNano 当前纳秒时间戳（运行时 id 生成用）。
func nowNano() int64 { return time.Now().UnixNano() }
