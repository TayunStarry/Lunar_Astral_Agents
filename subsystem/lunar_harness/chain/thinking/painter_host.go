package thinking

// painter_host.go 把月华的宿主能力适配给创作链调用的图片绘制智能体。

import "LunarSubsystem/LunarHarness/agents/Painter"

// PainterHost 图片绘制智能体执行环境的宿主实现。
type PainterHost struct{}

// PushImages 推送图片并返回可查看地址。
func (PainterHost) PushImages(images []string) []string {
	return pushAssistantImage(images, false)
}

// RegisterPainterImages 只登记图片资源并返回可查看地址。
// 不推送前端、不写近期记忆、不发 QQ，供静默产出等只需要句柄的调用使用。
func RegisterPainterImages(images []string) []string {
	canonical := make([]string, 0, len(images))
	for _, image := range images {
		canonical = append(canonical, registerPushedImage(image))
	}
	if len(canonical) == 0 {
		return images
	}
	return canonical
}

// VisionCopy 生成视觉副本并返回像素宽高。
func (PainterHost) VisionCopy(relPath, tag string) (int, int, error) {
	_, _, width, height, err := visionCopy(relPath, tag, 0)
	return width, height, err
}

// VisionReference 生成思维链消费的视觉读取引用。
func (PainterHost) VisionReference(handle, blobPath string) string {
	return visionReference(handle, blobPath)
}

// ReadImageBytes 把来源地址解析为图片字节。
func (PainterHost) ReadImageBytes(source string) ([]byte, error) {
	return resolveImageBytes(source)
}

// 确保宿主实现满足图片绘制智能体的环境契约。
var _ Painter.Env = PainterHost{}
