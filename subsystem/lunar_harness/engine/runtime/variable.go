package runtime

// ==== 常量与变量集中区 ====

import (
	"time"
)

// ServiceName 模块标识（日志分类据此归入「生活」）。
const ServiceName = "Runtime"

// nowNano 当前纳秒时间戳（运行时 id 生成用）。
func nowNano() int64 { return time.Now().UnixNano() }
