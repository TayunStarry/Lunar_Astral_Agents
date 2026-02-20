package processor

// MessageValid 检查文本消息是否有效
func (class TextMessage) MessageValid() bool {
	return len(class.Text) > 0
}

// MessageValid 检查图片消息是否有效
func (class ImageMessage) MessageValid() bool {
	return len(class.ImageURL.URL) > 0
}

// ProcessValid 检查图片对象参数是否有效
func (class ImageObjectParameter) ProcessValid() bool {
	return len(class.URL) > 0 || len(class.File) > 0
}

// ProcessValid 检查图片URL是否有效
func (class ImageURL) ProcessValid() bool {
	return len(class.URL) > 0
}

// MessageValid 检查处理结果是否有效
func (class ProcessResult) MessageValid() bool {
	return len(class) > 0
}

// MessageValid 检查知识库消息是否有效
func (class KnowledgeMessage) MessageValid() bool {
	return len(class.Content) > 0
}

// MessageValid 检查多模态消息是否有效
func (class MultimodalMessage) MessageValid() bool {
	return len(class.Content) > 0
}

// MessageValid 检查基础消息是否有效
func (class BaseMessage) MessageValid() bool {
	return len(class.Content) > 0
}
