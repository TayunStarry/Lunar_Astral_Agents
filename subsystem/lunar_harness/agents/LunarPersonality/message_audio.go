package LunarPersonality

// message_audio.go 消息接入阶段的音频处理：先转写，再入队。
//
// 前端语音以 input_audio / audio_url 内容项到达时，这里同步完成 audio_transcriber
// 转写并登记悬空句柄，随后把内容项替换为 audio_handle。这样：
//   - 音频本体不会落盘，聊天缓存只保留句柄与转写描述；
//   - 未读上下文直接携带转写文本，月华无需额外解码音频；
//   - 句柄随消息缓存保存，用户引用该消息时才有机会被再次读取。

import (
	"strings"
)

// ingestBatchAudio 批量处理消息中的音频内容项，返回替换后的消息与生成的句柄。
func ingestBatchAudio(messages []PostMessage) ([]PostMessage, []string) {
	handles := make([]string, 0, 4)
	if len(messages) == 0 {
		return messages, handles
	}
	out := make([]PostMessage, 0, len(messages))
	for index := range messages {
		message := messages[index]
		items := asItems(message.Content)
		if len(items) == 0 {
			out = append(out, message)
			continue
		}
		converted := make([]ContentItem, 0, len(items))
		changed := false
		for _, item := range items {
			switch item.Type {
			case ContentInputAudio:
				if item.InputAudio == nil {
					converted = append(converted, item)
					continue
				}
				format := strings.TrimSpace(item.InputAudio.Format)
				if format == "" {
					format = "wav"
				}
				source := "data:audio/" + format + ";base64," + item.InputAudio.Data
				replacement, handle := ingestAudioItem(source, "语音.wav", clientUploadOrigin)
				if handle != "" {
					converted = append(converted, replacement)
					changed = true
				} else {
					converted = append(converted, item)
				}
				// 每个音频位保留一个槽（失败为空串），前端按顺序回填句柄
				handles = append(handles, handle)
			case ContentAudioURL:
				if item.AudioURL == nil {
					converted = append(converted, item)
					continue
				}
				source := item.AudioURL.URL
				replacement, handle := ingestAudioItem(source, mediaName(source, ".wav"), clientUploadOrigin)
				if handle != "" {
					converted = append(converted, replacement)
					changed = true
				} else {
					converted = append(converted, item)
				}
				handles = append(handles, handle)
			default:
				converted = append(converted, item)
			}
		}
		if changed {
			message.Content = converted
		}
		out = append(out, message)
	}
	return out, handles
}

// ingestAudioItem 单个音频内容项 → 悬空句柄内容项。
func ingestAudioItem(source, name, origin string) (ContentItem, string) {
	handle, text, err := ingestAudioSource(source, name, origin)
	if err != nil || handle == "" {
		return ContentItem{}, ""
	}
	return audioHandleContextItem(handle, text, name), handle
}
