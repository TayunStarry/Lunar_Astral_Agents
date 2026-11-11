package ManipulateFiles

// type.go 文件操纵的输入输出。

// Request 一次文件操纵任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
}

// Result 一次文件操纵结果。
type Result struct {
	// Summary 执行摘要
	Summary string
	// Handles 本次涉及的资源句柄
	Handles []string
	// Changed 实际发生变化的相对路径
	Changed []string
	// Evidence 供审查使用的有界变更证据
	Evidence []string
}
