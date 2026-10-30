package BilibiliViewer

// events.go 哔哩观影的开始前 / 结束前钩子占位。
//
// 私有 js 运行时不再在生活链路上挂事件订阅（原 before_life_starts / before_life_ends
// 由插件订阅器消费），因此这里保持「始终允许开始、结束仅记录」的默认行为，
// 调用方无需改动。

// dispatchBeforeLifeStarts 观影开始前：始终允许开始。
func dispatchBeforeLifeStarts(cfg Config) (bool, string) {
	_ = cfg
	return true, ""
}

// dispatchBeforeLifeEnds 观影结束前（纯触发器）。
func dispatchBeforeLifeEnds(r *runner, result, reason string) {
	_ = r
	_ = result
	_ = reason
}
