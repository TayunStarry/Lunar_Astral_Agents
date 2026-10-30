package runtime

// ==== 常量与变量集中区 ====

import (
	"time"
)

// ServiceName 模块标识（日志分类据此归入「生活」）。
const ServiceName = "Runtime"

// EngineBuild 运行时构建标记：每次改动沙箱绑定/回调执行逻辑时递增，供运行实例自证版本。
const EngineBuild = "2026-10-30"

// priorityUnset 事件订阅器未设置优先级时的标记（排序时视作最大的优先级）。
const priorityUnset = -1

// nowNano 当前纳秒时间戳（运行时 id 生成用）。
func nowNano() int64 { return time.Now().UnixNano() }
