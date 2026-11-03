package music

// type.go 演奏者音乐渲染链路的类型定义。
// 链路：音乐描述 Brief → MIDI-LLM 的 AMT token → 标准 MIDI → fluidsynth 渲染 WAV。

// Brief 主模型产出的音乐内容描述（第二阶段的唯一输入）。
type Brief struct {
	// Title 作品标题
	Title string `json:"title"`
	// Prompt 交给 MIDI-LLM 的英文音乐描述（风格 / 情绪 / 乐器 / 篇幅）
	Prompt string `json:"prompt"`
	// Style 风格（如 抒情、激昂、欢快、神秘）
	Style string `json:"style"`
	// Mood 情绪
	Mood string `json:"mood"`
	// Tempo 速度（BPM）
	Tempo int `json:"tempo"`
	// Key 调式（如 C、G、Am）
	Key string `json:"key"`
	// Meter 拍号（如 4/4、3/4）
	Meter string `json:"meter"`
	// Bars 篇幅（小节数）
	Bars int `json:"bars"`
	// Instruments 乐器配置（主奏 / 伴奏 / 低音）
	Instruments []string `json:"instruments"`
	// Structure 段落结构
	Structure string `json:"structure"`
	// Summary 一句话作品简介（用于对话者转述）
	Summary string `json:"summary"`
}

// RenderResult 一次后端演奏渲染的结果。
type RenderResult struct {
	// AudioPath WAV 绝对路径（渲染失败时为空）
	AudioPath string
	// AudioURL 前端可直接播放的地址（/file/read/audios/xxx.wav）
	AudioURL string
	// DurationSeconds 音频时长（秒，未知时为 0）
	DurationSeconds float64
	// Source 乐谱来源：model=音乐模型输出
	Source string
}

// noteEvent MIDI 单音事件（内部类型）。
type noteEvent struct {
	// Tick 起始时间（PPQ 480 的 tick）
	Tick int
	// Duration 持续 tick
	Duration int
	// Note MIDI 音高
	Note int
	// Velocity 力度
	Velocity int
}

// midiVoice 单个声部（内部类型）。
type midiVoice struct {
	// Name 声部名
	Name string
	// Program GM 音色号（0-127）
	Program int
	// Channel MIDI 通道（0-15，鼓声部固定 9）
	Channel int
	// Notes 音符事件
	Notes []noteEvent
}

// parsedScore 解析后的乐谱（内部类型）。
type parsedScore struct {
	// Title 标题
	Title string
	// Tempo BPM
	Tempo int
	// MeterNum 拍号分子
	MeterNum int
	// MeterDen 拍号分母
	MeterDen int
	// Voices 声部列表
	Voices []midiVoice
}
