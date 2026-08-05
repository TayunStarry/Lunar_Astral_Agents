package learner

import (
	"sync"
	"websearch"
)

// ==== 全局变量 ====

// runtime 全局学习者运行时实例（单例）
var runtime *LearnerRuntime

// runtimeMutex 保护运行时的并发访问
var runtimeMutex sync.Mutex

// ==== 调试日志函数 ====

// debugLog 学习者调试日志回调，桥接到 websearch 子系统
func debugLog(format string, args ...interface{}) {
	if runtime != nil && runtime.system != nil && runtime.system.DebugLog != nil {
		runtime.system.DebugLog(format, args...)
	}
}

// ==== 默认配置 ====

// defaultWebSearchConfig 返回默认的网络检索配置（开启智能学习模式）
func defaultWebSearchConfig() websearch.Config {
	return websearch.Config{
		Simple: websearch.SimpleConfig{
			MaxResults: 10,
		},
		Webpage: websearch.WebpageConfig{
			MaxResults:       30,
			FetchContent:     true,
			FetchTimeout:     10,
			MaxContentLength: 2000,
		},
		Depth: websearch.DepthConfig{
			Enabled:       true,
			MaxSubQueries: 6,
			MaxGapRounds:  5,
		},
		KnowledgeVector: websearch.KnowledgeVectorConfig{
			Enabled: true,
		},
	}
}