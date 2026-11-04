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

// Review 一次模型输出的客观体检报告（生成端自检，不含渲染）。
type Review struct {
	// Tokens 有效 MIDI token 数
	Tokens int
	// Notes 解析出的音符数
	Notes int
	// Malformed 无法解析的三连音事件数
	Malformed int
	// Instruments 使用到的 GM 乐器编号（按编号升序）
	Instruments []int
	// DistinctPitches 不同音高数量
	DistinctPitches int
	// MaxOverlap 任一时刻同时发声的最大音符数
	MaxOverlap int
	// SpanSeconds 乐谱首音到末音结束的时长（秒）
	SpanSeconds float64
	// NotesPerSecond 音符密度（音符数 / 秒）
	NotesPerSecond float64
	// Issues 命中的问题码（可枚举，空表示通过）
	Issues []string
	// Score 候选排序分（越低越好）
	Score float64
}

// AudioReport 渲染产物的自检报告（渲染端）。
type AudioReport struct {
	// Seconds 音频时长（秒）
	Seconds float64
	// Peak 峰值（0-1 满量程）
	Peak float64
	// RMS 均方根（0-1 满量程）
	RMS float64
	// ClippedRatio 削波采样占比（0-1）
	ClippedRatio float64
	// Issues 命中的问题码（空表示通过）
	Issues []string
	// OK 是否通过基础可听性检查
	OK bool
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
