package ltpx

import "sync"

// ==== 调用超时与保留参数 ====
const (
	// ResultChannelBuffer 单次待定调用的结果通道缓冲
	ResultChannelBuffer = 1
	// MaxWaitSecond 单次 await 的兜底等待上限（秒），覆盖长时间运行的联网搜索等任务
	MaxWaitSecond = 30 * 60
)

// ==== 全局 Broker 持有 ====

// brokerMutex 保护当前注册的 Broker 实例
var brokerMutex sync.RWMutex

// currentBroker LTPX 协调实现（由装配层注入琉璃实现；未注入时全部接口返回离线态）
var currentBroker Broker

// SetBroker 注入 LTPX 协调实现（装配期调用一次）
func SetBroker(b Broker) {
	brokerMutex.Lock()
	currentBroker = b
	brokerMutex.Unlock()
}

// GetBroker 获取当前 LTPX 协调实现；未注入时返回 nil
func GetBroker() Broker {
	brokerMutex.RLock()
	defer brokerMutex.RUnlock()
	return currentBroker
}
