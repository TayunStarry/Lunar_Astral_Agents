package search

// variable.go 全局变量与常量集中声明（类型定义见 type.go）。

import "sync"

const (
	// readPageMaxSeconds 指定页面读取的独立预算上限（秒）
	readPageMaxSeconds = 180
	// readURLInstruction 未给出具体诉求时，单页提炼使用的默认指令
	readURLInstruction = "打开这个页面，提炼内容摘要"
)

var (
	// runMu 串行化网络搜索运行（共享浏览器窗口资源与模型切换租约）
	runMu sync.Mutex
)
