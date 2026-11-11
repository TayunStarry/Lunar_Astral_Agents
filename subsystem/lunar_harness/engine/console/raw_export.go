package console

// raw_export.go 原始日志导出的兼容入口。
// 实现位于 logbuf 子包（server / debugsnap 直接依赖子包）。

import (
	"LunarSubsystem/LunarHarness/engine/console/logbuf"
)

// BuildRawLogsJSONL 把「原始日志」缓冲序列化为 JSON Lines，返回文本与记录条数。
func BuildRawLogsJSONL() (string, int) { return logbuf.BuildRawLogsJSONL() }
