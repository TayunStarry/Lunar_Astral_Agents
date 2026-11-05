package napcat

// send.go 文本与图片消息发送。

import (
	"strings"
)

// SendPrivateTextMessage 发送私聊文本消息（原文整条发送，不做切片）
func SendPrivateTextMessage(userID int64, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return postMessage("/send_private_msg", map[string]any{"user_id": userID}, textSegment(content))
}

// SendGroupTextMessage 发送群文本消息：
//   - 内容中的 [对 xxx 说] 转换为真实 @ 段（按群成员名称缓存解析，可 @ 成员 / 全体成员）
//   - replyToUserID 非零时在消息最前追加对该成员的 @（被 @ 触发的回应礼节性 @ 回对方）
//   - quoteMessageID 非零时在最前追加 reply 段，引用被回应的那条原消息（随机触发的定位手段）
func SendGroupTextMessage(groupID int64, content string, replyToUserID int64, quoteMessageID int64) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return postMessage("/send_group_msg", map[string]any{"group_id": groupID},
		buildOutboundSegments(groupID, content, replyToUserID, quoteMessageID))
}

// SendPrivateImageMessage 发送私聊图片消息（base64 数据或 http 链接）
func SendPrivateImageMessage(userID int64, images []string) error {
	segments, cleanup := imageSegments(images)
	defer cleanup()
	return postMessage("/send_private_msg", map[string]any{"user_id": userID}, segments)
}

// SendGroupImageMessage 发送群图片消息（base64 数据或 http 链接）
func SendGroupImageMessage(groupID int64, images []string) error {
	segments, cleanup := imageSegments(images)
	defer cleanup()
	return postMessage("/send_group_msg", map[string]any{"group_id": groupID}, segments)
}

// postMessage 组装消息段并调用发送接口
func postMessage(action string, target map[string]any, segments []map[string]any) error {
	params := map[string]any{"message": segments}
	for key, value := range target {
		params[key] = value
	}
	_, err := Default().callAPI(action, params)
	return err
}

// normalizeBase64Image 规整图片数据：剥掉 "data:image/xxx;base64," 前缀
// NapCat 的 base64:// 仅接受纯 base64，而本系统的图片数据统一携带 data URI 前缀
func normalizeBase64Image(img string) string {
	if idx := strings.Index(img, "base64,"); idx >= 0 {
		return img[idx+len("base64,"):]
	}
	return img
}
