package ManipulateArchive

// type.go 归档操纵的输入输出。

// Request 一次归档操纵任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
}

// Result 一次归档操纵结果。
type Result struct {
	Summary  string
	Handles  []string
	Changed  []string
	Evidence []string
}
