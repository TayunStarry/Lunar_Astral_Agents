package WebCrawler

// util.go 抓取执行体的公共小工具（域名 / 日志）。
// 文本裁剪统一由 internal/textutil 提供，本包不再保留转发包装。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// DomainOf 导出包装，供链路层标注来源域名
func DomainOf(raw string) string { return domainOf(raw) }

// EngineDisplayName 导出包装，供链路层报告叙述
func EngineDisplayName(name string) string { return engineDisplayName(name) }

// loggerWarn 统一抓取侧警告日志
func loggerWarn(format string, args ...any) {
	logger.SubWarn("CrystalAstral", "WebSearch", format, args...)
}
