package netsource

// variable.go 回源与本地资源定位的全局常量与状态

import (
	"sync"
	"time"
)

const (
	// LocalFileReadPrefix 本机资源读取路由前缀（与 resource.FileReadPrefix 保持一致）
	LocalFileReadPrefix = "/file/read/"

	// userAgent 回源请求的默认 UA（部分图床 / CDN 会拒绝无 UA 的请求）
	userAgent = "LunarAstral/1.0"

	// maxRedirects 回源允许的最大重定向次数
	maxRedirects = 10
)

const (
	// addressTimeout 单次公网出口位置查询的超时
	addressTimeout = 6 * time.Second

	// addressRetryInterval 位置查询失败后的重试间隔
	addressRetryInterval = 10 * time.Minute

	// addressModule 位置查询告警的日志模块名
	addressModule = "LunarHarness"
)

var (
	// defaultAddress 位置查询失败时的兜底地址（省份、城市）
	defaultAddress = []string{"江苏省", "南京市"}

	// addressMu 保护地址缓存与查询时刻
	addressMu sync.Mutex

	// addressAttemptAt 最近一次外网查询的时刻（失败也记录，避免每次都重试）
	addressAttemptAt time.Time
)
