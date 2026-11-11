package tickloop

import "time"

const (
	// defaultModule 未注入日志模块名时的默认值。
	defaultModule = "TickLoop"
	// idlePollInterval WaitIdle 的轮询间隔。
	idlePollInterval = 200 * time.Millisecond
	// stopStepTimeout Stop 时等待后台步骤收尾的最长时间。
	stopStepTimeout = 10 * time.Second
)
