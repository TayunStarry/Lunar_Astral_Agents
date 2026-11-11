package library

// state.go 状态推导等纯函数。

// statusOf 推断文件名配色：运行失败红 > 运行成功绿 > 有操作记录黄 > 默认。
func statusOf(state *FileState) string {
	switch {
	case state.Failed:
		return StatusError
	case state.hasRun && state.runOK:
		return StatusOK
	case state.At > 0:
		return StatusActive
	default:
		return StatusNormal
	}
}

// boolInt 把布尔值转成 SQLite 的 0/1。
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
