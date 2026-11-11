package WebCrawler

// dump.go 网页抓取的导出：只读能力，不保留跨需求上下文。

import (
	"time"

	"LunarSubsystem/LunarHarness/protocol/agentdump"
)

// exportContext 网页抓取的上下文快照；dump 导出与智能修复的 Inspect 共用这一份。
func exportContext(includeImages bool) (any, error) {
	return map[string]any{
		"kind":          "web-crawler-context",
		"generated_at":  time.Now().Format(time.RFC3339),
		"message_count": 0,
		"note":          "网页抓取按次执行，不保留跨需求上下文；近期访问记录由统一句柄库的 url_master 维护",
		"config":        LoadConfig(),
		"tools": []map[string]string{
			{"name": ToolCrawl, "description": "多引擎降级检索并并发采集详情页正文"},
			{"name": ToolReadURL, "description": "打开指定页面并提取正文"},
		},
	}, nil
}

func init() {
	agentdump.Register(agentdump.Provider{
		ID:          "web_crawler",
		Label:       "网页抓取",
		Group:       "Go 智能体",
		Description: "网页抓取执行体的检索配置、引擎降级顺序与工具清单",
		Export:      exportContext,
	})
}
