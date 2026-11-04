package SearchCrawler

// util.go 抓取执行体的公共小工具（文本裁剪 / 域名 / 日志），
// 同时对外暴露搜索链与摘要智能体需要的少量纯函数。

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// truncateRunes 按字符截断并追加省略号（日志预览用）
func truncateRunes(s string, max int) string {
	return agentbase.TruncateRunes(s, max)
}

// TruncateRunes 导出包装，供链路层日志预览复用
func TruncateRunes(s string, max int) string { return truncateRunes(s, max) }

// hardCutRunes 硬截断并追加「已截断」标记（提示词注入用）
func hardCutRunes(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if max <= 0 || len(r) <= max {
		return string(r)
	}
	return string(r[:max-1]) + "…（已截断）"
}

// HardCutRunes 导出包装，供摘要智能体复用
func HardCutRunes(s string, max int) string { return hardCutRunes(s, max) }

// firstNonEmpty 返回第一个非空白字符串
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// FirstNonEmpty 导出包装，供链路层复用
func FirstNonEmpty(values ...string) string { return firstNonEmpty(values...) }

// DomainOf 导出包装，供链路层标注来源域名
func DomainOf(raw string) string { return domainOf(raw) }

// EngineDisplayName 导出包装，供链路层报告叙述
func EngineDisplayName(name string) string { return engineDisplayName(name) }

// loggerWarn 统一抓取侧警告日志
func loggerWarn(format string, args ...any) {
	logger.SubWarn("CrystalAstral", "WebSearch", format, args...)
}
