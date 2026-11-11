package CodeExecution

// type.go 代码执行的输入输出。

// Request 一次代码执行 / 解析任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
}

// Result 一次代码执行结果。
type Result struct {
	Summary string
	Handles []string
}
