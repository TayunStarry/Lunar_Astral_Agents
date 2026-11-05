package search

// type.go 全部类型定义（结构体、接口、别名）集中于此。

import (
	"LunarSubsystem/LunarHarness/chain/search/crawler"
)

// reportInput 输出收口所需的一次搜索快照
type reportInput struct {
	Instruction    string                // 原始指令
	Query          string                // 最终检索词
	EngineName     string                // 胜出引擎中文名（必应/百度/搜狗）
	Engine         crawler.PageExtract   // 结果页提取（已过滤字典站并去重）
	SearchPageText string                // 搜索页完整滚动后的正文
	Digest         string                // 摘要智能体产出
	Pages          []crawler.PageCapture // 各详情页原始提取
	Interrupted    bool                  // 是否在中途被关闭
	Elapsed        string                // 总耗时（人类可读）
}
