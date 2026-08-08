package learner

import "sync"

// ==== 全局变量 ====

// runtimeMutex 保护初始化状态的并发访问
var runtimeMutex sync.Mutex

// learnerInitialized 标记学习者是否已初始化
var learnerInitialized bool
