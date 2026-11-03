package tickloop

import "time"

const (
	// defaultModule 未注入日志模块名时的默认值。
	defaultModule = "TickLoop"
	// idlePollInterval WaitIdle 的轮询间隔。
	idlePollInterval = 200 * time.Millisecond
)
