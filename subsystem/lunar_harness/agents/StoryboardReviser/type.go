package StoryboardReviser

// type.go 分镜规划-修订的类型定义。

// Shot 审查稿中的单个分镜。
type Shot struct {
	Act        string `json:"act,omitempty"`
	Scene      string `json:"scene,omitempty"`
	Narration  string `json:"narration"`
	Continuity string `json:"continuity,omitempty"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio,omitempty"`
}

// Request 修订输入：用户确认后的完整审查稿。
type Request struct {
	// Title 作品标题
	Title string
	// Style 画风约束
	Style string
	// Ratio 任务默认画幅
	Ratio string
	// Shots 用户确认后的分镜（顺序即最终顺序）
	Shots []Shot
}

// Result 修订结果。
type Result struct {
	// Title 作品标题（用户标题优先）
	Title string
	// Shots 整理后的分镜
	Shots []Shot
}

// revisedLine 模型输出的单行结构（标题行与分镜行共用）。
type revisedLine struct {
	Title      string `json:"title"`
	Act        string `json:"act"`
	Scene      string `json:"scene"`
	Narration  string `json:"narration"`
	Continuity string `json:"continuity"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio"`
}
