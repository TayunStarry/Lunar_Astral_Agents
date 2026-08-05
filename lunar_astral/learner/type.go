package learner

import "websearch"

// LearnerRuntime 学习者智能体 Goja 运行时封装
// 负责管理 websearch 子系统实例，并暴露 Goja 绑定函数供 TS 层调用
type LearnerRuntime struct {
	system *websearch.System // 网络检索子系统实例（内含 SearchLearner）
}