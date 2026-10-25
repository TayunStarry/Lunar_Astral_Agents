package QQMessenger

// variable.go QQ 投递智能体的全局变量与机制参数。

import (
	"net/http"
	"sync"
	"time"
)

const (
	// roleLabel 该智能体在「核心智能 实况」中的角色标签
	roleLabel = "QQ投递"
	// defaultModelName 未配置多模态模型时的兜底模型名
	defaultModelName = "system-multimodal"
	// maxDirectoryEntries 提示词里最多列出多少个群 / 好友，避免撑爆上下文
	maxDirectoryEntries = 80
	// maxSendImages 单次消息最多携带的图片数（超过则分批发送）
	maxSendImages = 9
	// maxSendFiles 单次请求最多投递的文件数
	maxSendFiles = 10
	// loopMaxRetries 模型重复输出时的额外重试次数
	loopMaxRetries = 2
)

// 模型端点缓存（只解析一次）
var (
	modelOnce sync.Once
	modelName string
	modelURL  string
	modelKey  string
)

// httpClient 单次规划调用的 HTTP 客户端
var httpClient = &http.Client{Timeout: 90 * time.Second}

// runMu 串行化投递：同一时间只处理一个需求，避免并发写 QQ 时报告互相覆盖
var runMu sync.Mutex

// 模型实况观察者：由 app 注入控制台的 StreamBegin/Delta/End 与计量。
var (
	observerMu sync.Mutex
	obsBegin   func(role string)
	obsDelta   func(role, kind, text string)
	obsEnd     func(role string)
	obsMeter   func(prompt, completion int64, generation time.Duration, estimated bool)
)
