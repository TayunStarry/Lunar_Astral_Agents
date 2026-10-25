package lunar

import (
	"strings"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
)

// LiteImageFile 处理未读消息中的媒体内容
func LiteImageFile() {
	unreadMu.Lock()
	messages := append([]PostMessage(nil), unreadContext...)
	unreadMu.Unlock()

	for index := range messages {
		if _, ok := messages[index].Content.(string); ok {
			continue
		}
		items := asItems(messages[index].Content)
		if len(items) == 0 {
			continue
		}
		newItems := make([]ContentItem, 0, len(items))
		for _, item := range items {
			switch item.Type {
			case ContentText:
				newItems = append(newItems, item)
			case ContentVideoURL:
				url := ""
				if item.VideoURL != nil {
					url = item.VideoURL.URL
				}
				newItems = append(newItems, ContentItem{Type: ContentText, Text: resourceVideoOrFallback(url)})
			case ContentAudioURL:
				url := ""
				if item.AudioURL != nil {
					url = item.AudioURL.URL
				}
				newItems = append(newItems, ContentItem{Type: ContentText, Text: resourceAudioOrFallback(url)})
			case ContentInputAudio:
				if item.InputAudio == nil {
					continue
				}
				format := item.InputAudio.Format
				if format == "" {
					format = "wav"
				}
				source := "data:audio/" + format + ";base64," + item.InputAudio.Data
				newItems = append(newItems, ContentItem{Type: ContentText, Text: resourceAudioOrFallback(source)})
			case ContentImageURL:
				if item.ImageURL == nil {
					continue
				}
				source := item.ImageURL.URL
				lower := strings.ToLower(source)
				if strings.HasPrefix(lower, "data:video/") || hasVideoExt(lower) {
					newItems = append(newItems, ContentItem{Type: ContentText, Text: resourceVideoOrFallback(source)})
					continue
				}
				animated, err := isAnimatedImageSource(source)
				if err != nil {
					logger.Warn(lunarCoreModule, "[动态图判定失败]: %v", err)
				}
				if err == nil && animated {
					tag := handleFromVisionURL(source)
					if tag == "" {
						if admitted := admitImageDataURI(source, "image"+guessImageExt(source)); admitted != nil {
							tag = admitted.Handle
						}
					}
					if tag == "" {
						tag = mediaName(source, guessImageExt(source))
					}
					newItems = append(newItems, ContentItem{Type: ContentText, Text: tag})
					continue
				}
				newItems = append(newItems, item)
			default:
				newItems = append(newItems, item)
			}
		}
		messages[index].Content = newItems
	}

	unreadMu.Lock()
	unreadContext = messages
	unreadMu.Unlock()
}

// mediaName 按来源推断文件名
func mediaName(source, fallbackExt string) string {
	lower := strings.ToLower(source)
	if strings.HasPrefix(lower, "data:image/") || strings.HasPrefix(lower, "data:video/") || strings.HasPrefix(lower, "data:audio/") {
		head := lower[len("data:"):]
		if cut := strings.IndexAny(head, ";,"); cut >= 0 {
			head = head[:cut]
		}
		if cut := strings.LastIndex(head, "/"); cut >= 0 {
			ext := head[cut+1:]
			if ext == "jpeg" {
				ext = "jpg"
			}
			return "media." + ext
		}
	}
	path := pathPartOf(lower)
	base := path[strings.LastIndex(path, "/")+1:]
	if base != "" && strings.LastIndex(base, ".") > 0 {
		return base
	}
	return "media" + fallbackExt
}

// resourceVideo 视频资源化（只发句柄）
func resourceVideo(source, name, origin string) string {
	media := admitMedia(source, name, origin, "V")
	if media == nil {
		return ""
	}
	return media.Handle
}

// resourceVideoOrFallback 视频资源化包装：句柄优先，入库失败退化为文件名
func resourceVideoOrFallback(source string) string {
	name := mediaName(source, ".mp4")
	origin := "channel"
	if strings.HasPrefix(source, "data:") {
		origin = "client-upload"
	}
	if handle := resourceVideo(source, name, origin); handle != "" {
		return handle
	}
	return name
}

// understandAudio 理解音频（ASR 转写）
func understandAudio(source string) (string, error) {
	cacheable := !strings.HasPrefix(source, "data:")
	if cacheable {
		if cached := getPromptFromKnowledge(source); cached != "" {
			return cached, nil
		}
	}
	wav, err := multimodal.AudioToWavBase64(source)
	if err != nil || wav == "" {
		logger.Warn(lunarCoreModule, "[语音] 音频转换失败: %v", err)
		return "", err
	}
	transcript, err := transcribeAudio(wav)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(transcript) == "" {
		return "", nil
	}
	text := "【语音转写】" + transcript
	if cacheable {
		savePromptToKnowledge(source, text)
	}
	return text, nil
}

// transcribeAudio 调用语音识别模型
func transcribeAudio(wavBase64 string) (string, error) {
	body := map[string]any{
		"model": config.AgentASRModel,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_audio",
				"input_audio": map[string]any{
					"data":   wavBase64,
					"format": "wav",
				},
			}},
		}},
		"temperature": 0,
	}
	result, err := chatCompletionJSON(body)
	if err != nil {
		logger.Warn(lunarCoreModule, "[语音] 语音转写请求失败: %v", err)
		return "", err
	}
	choices, _ := result["choices"].([]any)
	if len(choices) == 0 {
		return "", nil
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	raw, _ := message["content"].(string)
	const marker = "<asr_text>"
	if index := strings.Index(raw, marker); index >= 0 {
		return strings.TrimSpace(raw[index+len(marker):]), nil
	}
	return strings.TrimSpace(raw), nil
}

// resourceAudio 音频资源化：转写 → 登记 → 写回结论
func resourceAudio(originalSource, name, origin string) string {
	media := admitMedia(originalSource, name, origin, "A")
	if media == nil {
		return ""
	}
	if entry := getResourceEntry(media.Handle); entry != nil && entry.Summary != "" {
		return media.Handle + entry.Summary
	}
	transcript, err := understandAudio(originalSource)
	if err != nil {
		return media.Handle + asrFallbackText
	}
	if transcript == "" {
		return media.Handle + asrFallbackText
	}
	setResourceSummary(media.ID, transcript, nil)
	return media.Handle + transcript
}

// resourceAudioOrFallback 音频资源化包装：句柄优先，入库失败退化为文件名
func resourceAudioOrFallback(source string) string {
	name := mediaName(source, ".wav")
	origin := "channel"
	if strings.HasPrefix(source, "data:") {
		origin = "client-upload"
	}
	if text := resourceAudio(source, name, origin); text != "" {
		return text
	}
	if text, err := understandAudio(source); err == nil && text != "" {
		return text
	}
	return name
}
