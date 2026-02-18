package processor

// ProcessValid 检查文本消息是否有效
func (class TextMessage) ProcessValid() bool {
	return len(class.Text) > 0
}

// ProcessValid 检查图片消息是否有效
func (class ImageMessage) ProcessValid() bool {
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

// ProcessValid 检查处理结果是否有效
func (class ProcessResult) ProcessValid() bool {
	return len(class) > 0
}
