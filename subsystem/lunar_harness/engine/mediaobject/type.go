// Package mediaobject 定义「多模态对象」的唯一结构。
//
// 写入侧（QQ 适配器、HTTP/HTTPS 前端）与接收侧（POST /write/message）共用本包：
// 写入侧只按来源方声明填写类型、来源与元数据，不做内容嗅探；
// 接收侧只做校验与拒绝，不做任何类型推断。
package mediaobject

// Object 一个多模态对象。
//
// 信封类型（content item 的 type，见 Envelope* 常量）负责传输形态，
// Object.Type 负责语义类型：静态图 / 动图 / 视频 / 音频各有独立取值，
// 后端不得再根据 URL、扩展名或字节内容反推类型。
type Object struct {
	// Type 语义类型（见 Type* 常量）
	Type string `json:"type"`
	// Source 来源（见 Source* 常量）
	Source string `json:"source"`
	// URL 引用：http(s) 地址、data URI、file:// 或本地路径
	URL string `json:"url,omitempty"`
	// Data 内联音频的原始 base64（仅 input_audio 使用）
	Data string `json:"data,omitempty"`
	// Format 内联音频容器格式（仅 input_audio 使用，如 wav / mp3）
	Format string `json:"format,omitempty"`
	// MIME 来源方声明的媒体类型；来源方无法声明时留空
	MIME string `json:"mime,omitempty"`
	// Name 展示文件名
	Name string `json:"name,omitempty"`
	// Width 像素宽度（来源方声明时填写）
	Width int `json:"width,omitempty"`
	// Height 像素高度（来源方声明时填写）
	Height int `json:"height,omitempty"`
	// DurationMS 时长毫秒（来源方声明时填写）
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Bytes 字节大小（来源方声明时填写）
	Bytes int64 `json:"bytes,omitempty"`
}

// IsMediaKind 判断是否为本协议定义的媒体语义类型
func IsMediaKind(kind string) bool {
	switch kind {
	case TypeImageStatic, TypeImageAnimated, TypeVideo, TypeAudio:
		return true
	default:
		return false
	}
}
