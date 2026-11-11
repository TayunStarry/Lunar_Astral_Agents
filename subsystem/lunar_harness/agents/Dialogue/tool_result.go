package Dialogue

// tool_result.go 工具执行结果的公共类型、能力注入面与序列化工具。
//
// 工具执行器归属智能体；但执行器需要的创作 / 检索 / 生活 / 宿主推送能力
// 必须由装配层以函数变量注入，本包不 import 任何 chain 包。

import (
	"encoding/json"

	"LunarSubsystem/LunarHarness/protocol/agentdispatch"
)

// ToolResult 工具执行结果：文本 + 可选主帧图片。
type ToolResult struct {
	Text  string
	Image string
}

// 能力注入面：由装配层（app）在启动前赋值。
// 未注入时执行器返回明确的失败原因，不静默降级。
var (
	// WebSearch 联网检索（检索链）
	WebSearch func(query string) (string, error)
	// RequestLifeEvent 登记强制生活事件（生活链），handles 为任务涉及的资源句柄
	RequestLifeEvent func(eventID, instruction string, handles []string) bool
	// PushImages 推送图片消息并返回可引用句柄（月华宿主）
	PushImages func(images []string) []string
	// PushMusic 推送可播放音乐消息（月华宿主）
	PushMusic func(title, summary string, duration float64, audio string) bool
	// VisionCopy 生成视觉副本并返回宽高（月华宿主）
	VisionCopy func(relPath, tag string) (width, height int, err error)
	// VisionReference 登记视觉参考描述（月华宿主）
	VisionReference func(handle, blobPath string) string
	// ReadImageBytes 读取图片本体（月华宿主）
	ReadImageBytes func(source string) ([]byte, error)
	// ResolveImageRef 解析图片句柄（月华宿主）
	ResolveImageRef func(raw string) (handle, source, desc string, ok bool)
	// SaveImage 落盘图片并返回可读地址（月华宿主）
	SaveImage func(name string, data []byte) (string, error)
)

// toolJSON 把统一输出原语序列化为工具返回文本。
func toolJSON(result agentdispatch.Result) ToolResult {
	return ToolResult{Text: result.JSON()}
}

// jsonText 把任意值序列化为 JSON 文本，失败时返回空串。
func jsonText(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

// safeJSON 把任意值序列化为 JSON 文本，失败时返回兜底值。
func safeJSON(value any, fallback string) string {
	if text := jsonText(value); text != "" {
		return text
	}
	return fallback
}
