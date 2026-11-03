package Musician

// type.go 演奏者对外结果。

// Result 一次演奏的产物：标题、描述、时长与可播放音频地址。
type Result struct {
	// Title 作品标题
	Title string
	// Description 推送卡片上的一行描述
	Description string
	// Duration 音频时长（秒）
	Duration float64
	// AudioURL 前端可直接播放的音频地址
	AudioURL string
	// Summary 回传给对话者的作品介绍
	Summary string
}
