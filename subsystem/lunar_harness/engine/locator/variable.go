package locator

// variable.go 定位器常量与缓存状态（业务逻辑不得写在本文件）

import (
	"sync"
	"time"
)

const (
	// indexTTL 目录索引的最长复用时间（外部写入可能绕过显式失效）
	indexTTL = 3 * time.Second
	// maxCandidateDisplay 歧义提示里最多列出的候选条数
	maxCandidateDisplay = 12
)

var (
	// indexMu 保护索引缓存
	indexMu sync.Mutex
	// indexCache 搜索根绝对路径 → 目录索引
	indexCache = map[string]*dirIndex{}
)
