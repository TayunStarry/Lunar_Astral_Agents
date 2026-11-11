package FileOperations

// type.go 文件操作的输入输出。

// Request 一次文件操作任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
}

// Result 一次文件操作结果。
type Result struct {
	Summary string
	Handles []string
	Changed []string
	// Folder 本次操作实际使用的项目目录（documents/<标题>）；无法推断时为空。
	Folder string
}
