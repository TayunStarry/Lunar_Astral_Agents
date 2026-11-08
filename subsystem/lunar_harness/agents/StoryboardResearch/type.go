package StoryboardResearch

// type.go 分镜规划-资料的类型定义。

// Result 资料准备结果。
type Result struct {
	// Analysis 为什么需要这些资料（可读文本）
	Analysis string
	// Queries 检索意图（已去空、去重并限制条数）
	Queries []string
}
