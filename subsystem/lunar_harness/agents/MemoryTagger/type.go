package MemoryTagger

// type.go 记忆标签智能体的请求与消息结构。

// Request 标签生成请求。
type Request struct {
	// Content 待生成标签的文本，或单张图片的引用（data URI / URL）
	Content string
	// IsImage 为 true 时按图片多模态格式请求
	IsImage bool
	// Orientation 图片识别取向（auto/emotion/text/color/appearance/species/posture/custom）
	Orientation string
	// Custom 自定义识别取向的参考文本
	Custom string
	// MediaRefs 动态图采样帧引用；非空时按帧序列请求
	MediaRefs []string
}

// chatMessage OpenAI 兼容聊天消息。
type chatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// chatContentPart 多模态消息内容片段。
type chatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

// imageURL 图片引用。
type imageURL struct {
	URL string `json:"url"`
}
