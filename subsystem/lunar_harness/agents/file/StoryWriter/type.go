package StoryWriter

// type.go 故事编写的输入输出与故事列表结构。

// Request 一次故事编写任务。
type Request struct {
	Instruction   string
	Handles       []string
	WorkspaceRoot string
	// Mode 上级指定的模式：auto / new / continue；为空按 auto 处理。
	Mode string
	// Story 上级指定的故事名，仅延续时有意义。
	Story string
	// StartedUnix 本次任务开始时间；用于识别本次任务内刚创建的故事并复用，
	// 避免重试时把同一个故事再新建一遍。
	StartedUnix int64
}

// Result 一次故事编写结果。
type Result struct {
	Summary string
	Folder  string
	Handles []string
	Changed []string
	// Mode 本次实际执行的模式：new / continue。
	Mode string
	// Story 本次实际对应的故事名；新建后由变更路径推断，无法推断时为空。
	Story string
}

// StoryInfo 已有故事的摘要，供调度智能体选择新建或延续。
type StoryInfo struct {
	Name        string `json:"name"`
	Dir         string `json:"dir"`
	Chapters    int    `json:"chapters"`
	LastChapter int    `json:"last_chapter"`
	Updated     string `json:"updated"`
	UpdatedUnix int64  `json:"updated_unix"`
}
