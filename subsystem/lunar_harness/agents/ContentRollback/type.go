package ContentRollback

// type.go 内容回滚的输入输出。

// Request 一次回滚任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
}

// Result 一次回滚结果。
type Result struct {
	Summary string
	Handles []string
	Changed []string
}
