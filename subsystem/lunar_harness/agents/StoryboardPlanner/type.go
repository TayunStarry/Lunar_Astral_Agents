package StoryboardPlanner

// type.go 分镜规划-拆解的类型定义。

// Shot 模型给出的单个分镜。
type Shot struct {
	// Index 全局序号（归一化时填写，不要求模型返回）
	Index     int    `json:"-"`
	Act       string `json:"act"`
	Scene     string `json:"scene"`
	Narration string `json:"narration"`
	// Continuity 本镜与上一镜的画面衔接
	Continuity string `json:"continuity"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio"`
	// Style 用户画风约束，仅作为绘图交接参数，不写入提示词
	Style string `json:"-"`
}

// Result 一次分镜规划的结构化结果。
type Result struct {
	// Title 作品标题
	Title string
	// Style 用户显式提交的画风约束
	Style string
	// Shots 分镜列表
	Shots []Shot
}

// Request 分镜规划输入。
type Request struct {
	// Story 故事正文
	Story string
	// Style 画风约束
	Style string
	// Ratio 统一画幅
	Ratio string
	// Research 资料准备阶段累积的参考资料
	Research string
	// OnThought 接收叙事推演的可读文本增量
	OnThought func(string)
	// OnShot 每解析出一镜时回调（供调用方推送逐镜事件）
	OnShot func(title string, shot Shot)
}

// plannedLine 模型输出的单行结构（标题行与分镜行共用）。
type plannedLine struct {
	Title      string `json:"title"`
	Act        string `json:"act"`
	Scene      string `json:"scene"`
	Narration  string `json:"narration"`
	Continuity string `json:"continuity"`
	Prompt     string `json:"prompt"`
	Ratio      string `json:"ratio"`
}
