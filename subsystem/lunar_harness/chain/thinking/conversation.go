package thinking

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/engine/console/logger"
	"LunarSubsystem/LunarHarness/engine/mediaobject"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	"LunarSubsystem/LunarHarness/engine/resource"
)

var (
	// groupPrefixRegex 群聊消息的 `[群聊: 群名]` 前缀（可缺省）
	groupPrefixRegex = regexp.MustCompile(`^\[群聊:\s*[^\]]*\]\s*`)
	userPrefixRegex  = regexp.MustCompile(`^\[用户:\s*([^\]]*)\]\s*[:：]\s*`)
	// 以下三个正则都由 engine/resource 的权威文法拼出，禁止另写类型码集合
	fileRefRegex    = regexp.MustCompile(resource.HandlePattern)
	noticeRegex     = regexp.MustCompile(`^\[已收录\]\s*(` + resource.HandlePattern + `)\s*(.*)$`)
	handlePathRegex = regexp.MustCompile(`^/file/read/(` + resource.HandlePattern + `)/?$`)
)

// newID 生成消息 ID（时间戳 + 随机后缀）
func newID() string {
	return fmt.Sprintf("%s%s", strings.ToLower(fmt.Sprintf("%x", time.Now().UnixNano()/1e6)), randomLetters())
}

func randomLetters() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var builder strings.Builder
	for i := 0; i < 6; i++ {
		builder.WriteByte(alphabet[rand.Intn(len(alphabet))])
	}
	return builder.String()
}

// splitUserPrefix 拆出 `[群聊: 群名][用户: 名字]: ` 前缀
//
// 群聊消息先去掉群名，再拆用户前缀；私聊只有用户前缀。
func splitUserPrefix(text string) (sender, body string) {
	rest := text
	if groupPrefixRegex.MatchString(rest) {
		rest = groupPrefixRegex.ReplaceAllString(rest, "")
	}
	match := userPrefixRegex.FindStringSubmatch(rest)
	if match == nil {
		return "", text
	}
	return strings.TrimSpace(match[1]), rest[len(match[0]):]
}

// pathPartOf 去掉地址中的查询串与锚点
func pathPartOf(source string) string {
	path := source
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	return path
}

// toDisplaySource 转成前端可查看原图的地址
func toDisplaySource(source string) string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "blob:") {
		return raw
	}
	path := pathPartOf(raw)
	if index := strings.Index(path, fileReadPath); index >= 0 {
		return path[index:]
	}
	if index := strings.Index(path, visionReadPath); index >= 0 {
		return fileReadPath + path[index+len(visionReadPath):]
	}
	if strings.HasPrefix(path, "multimedia/") {
		return fileReadPath + path
	}
	return raw
}

// localPathToReadURL 本地路径 → /file/read/ 地址（仅 multimedia/ 下）
func localPathToReadURL(source string) string {
	if source == "" {
		return ""
	}
	normalized := strings.ReplaceAll(source, "\\", "/")
	if cut := strings.IndexAny(normalized, "?#"); cut >= 0 {
		normalized = normalized[:cut]
	}
	marker := strings.LastIndex(strings.ToLower(normalized), "/multimedia/")
	rel := ""
	if marker >= 0 {
		rel = normalized[marker+1:]
	} else if strings.HasPrefix(strings.ToLower(normalized), "multimedia/") {
		rel = normalized
	}
	if rel == "" {
		return ""
	}
	return fileReadPath + rel
}

// fileNameOf 从地址/路径中取文件名
func fileNameOf(source string) string {
	path := pathPartOf(source)
	name := path[strings.LastIndex(path, "/")+1:]
	if name == "" {
		return "文件"
	}
	return name
}

// saveBase64Media 把 base64 媒体写入 multimedia/，返回 /file/read/ 地址
func saveBase64Media(base64Text, mime, kind string) string {
	raw := strings.TrimSpace(base64Text)
	if raw == "" {
		return ""
	}
	contentType := mime
	if strings.HasPrefix(raw, "data:") {
		if match := regexp.MustCompile(`^data:([^;,]+);base64,(.*)$`).FindStringSubmatch(raw); match != nil {
			contentType = match[1]
			raw = match[2]
		}
	}
	if kind != "voice" && kind != "video" && !strings.HasPrefix(contentType, "image/") {
		contentType = "image/png"
	}
	extMap := map[string]string{
		"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif", "image/bmp": "bmp",
		"audio/wav": "wav", "audio/x-wav": "wav", "audio/mpeg": "mp3", "audio/mp3": "mp3", "audio/flac": "flac", "audio/ogg": "ogg",
		"video/mp4": "mp4", "video/webm": "webm", "video/quicktime": "mov", "video/x-msvideo": "avi", "video/x-matroska": "mkv",
	}
	ext := extMap[contentType]
	if ext == "" {
		ext = "bin"
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		logger.Warn(lunarCoreModule, "[对话缓存] base64 解码失败: %v", err)
		return ""
	}
	name := fmt.Sprintf("multimedia/%s_%d_%s.%s", kind, time.Now().UnixMilli(), randomLetters(), ext)
	if _, err := persistence.SaveFileEx(name, persistence.SaveOptions{Overwrite: true}, bytes.NewReader(data)); err != nil {
		logger.Warn(lunarCoreModule, "[对话缓存] 媒体落盘失败: %v", err)
		return ""
	}
	return fileReadPath + name
}

// buildUserRecord 从用户消息内容构造展示记录
func buildUserRecord(content any) *CachedMessage {
	attachments := make([]CachedAttachment, 0)
	texts := make([]string, 0)
	quotes := make([]QuotedMessage, 0)
	sender := ""
	hasImage := false
	hasVoice := false

	pushAttachment := func(kind, source, label string) {
		src := toDisplaySource(source)
		if src == "" {
			return
		}
		if label == "" {
			label = fileNameOf(src)
		}
		attachments = append(attachments, CachedAttachment{Type: kind, Src: src, Label: label})
	}
	pushText := func(raw string) {
		parsedSender, parsedBody := splitUserPrefix(raw)
		if sender == "" && parsedSender != "" {
			sender = parsedSender
		}
		if parsedBody == "" {
			return
		}
		texts = append(texts, raw)
	}

	if text, ok := content.(string); ok {
		pushText(text)
	} else {
		for _, item := range asItems(content) {
			switch item.Type {
			case ContentText:
				pushText(item.Text)
			case ContentImageURL:
				if item.ImageURL != nil {
					pushAttachment("image", item.ImageURL.URL, "")
					hasImage = true
				}
			case ContentImageStatic:
				if item.ImageStatic != nil {
					pushAttachment("image", item.ImageStatic.Reference(), "")
					hasImage = true
				}
			case ContentImageAnimated:
				if item.ImageAnimated != nil {
					pushAttachment("image", item.ImageAnimated.Reference(), "")
					hasImage = true
				}
			case ContentVideo:
				if item.Video != nil {
					pushAttachment("video", item.Video.Reference(), "")
					hasImage = true
				}
			case ContentAudio:
				// 音频实体在接入阶段落盘并转写，缓存只记录语音分类
				if item.Audio != nil {
					attachments = append(attachments, CachedAttachment{Type: "audio", Label: "语音"})
					hasVoice = true
				}
			case ContentAudioHandle:
				if item.AudioHandle != nil {
					attachments = append(attachments, CachedAttachment{
						Type:   "audio",
						Label:  "语音",
						Handle: item.AudioHandle.Handle,
						Text:   item.AudioHandle.Text,
					})
					hasVoice = true
				}
			case ContentQuote:
				if item.Quote != nil {
					for _, quoted := range item.Quote.Messages {
						quotes = append(quotes, quoted)
						if strings.TrimSpace(quoted.AudioHandle) != "" {
							hasVoice = true
						}
					}
				}
			}
		}
	}

	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" && len(attachments) == 0 && len(quotes) == 0 {
		return nil
	}
	categories := make([]string, 0)
	if text != "" {
		categories = append(categories, "text")
	}
	if hasImage {
		categories = append(categories, "image")
	}
	if hasVoice {
		categories = append(categories, "voice")
	}
	if fileRefRegex.MatchString(text) {
		categories = append(categories, "file")
	}
	if len(categories) == 0 {
		categories = []string{"text"}
	}
	record := &CachedMessage{
		ID:         newID(),
		Role:       RoleUser,
		Categories: categories,
		Content:    text,
		Timestamp:  time.Now().UnixMilli(),
	}
	if sender != "" {
		record.Sender = sender
	}
	if len(attachments) > 0 {
		record.Attachments = attachments
	}
	if len(quotes) > 0 {
		record.Quotes = quotes
	}
	return record
}

// appendUserMessage 用户消息入缓存
func appendUserMessage(content any) {
	record := buildUserRecord(content)
	if record == nil {
		return
	}
	appendCachedMessage(*record)
	persistConversation()
}

// recordAssistantContext 记录一条助手文本/语音/动作消息
func recordAssistantContext(kind, content, audio string) {
	record := CachedMessage{
		ID:         newID(),
		Role:       RoleAssistant,
		Categories: []string{"text"},
		Content:    content,
		Timestamp:  time.Now().UnixMilli(),
	}
	switch kind {
	case "text":
		if audio != "" {
			record.Categories = []string{"text", "voice"}
			record.Audio = audio
		}
		if match := noticeRegex.FindStringSubmatch(strings.TrimSpace(record.Content)); match != nil {
			record.Categories = []string{"text", "file"}
			record.FileRef = match[1]
			// 句柄文法内含 5 个捕获组，标签是最后一个捕获组
			label := strings.TrimSpace(match[len(match)-1])
			if label == "" {
				label = match[1]
			}
			record.FileLabel = label
		}
	default:
		record.Categories = []string{kind}
	}
	appendCachedMessage(record)
}

// pushContext 推送上下文数据
func pushContext(kind, content, audio string, auto bool) bool {
	PushMessageFunc("context", PushContextData{Type: kind, Content: content, Audio: audio, Auto: auto})
	return true
}

// pushAssistantContext 推送助手上下文并写入缓存
func pushAssistantContext(kind, content, audio string, auto bool) bool {
	ok := pushContext(kind, content, audio, auto)
	recordAssistantContext(kind, content, audio)
	persistConversation()
	return ok
}

// PushAssistantMusic 推送一条音乐消息并写入缓存。
// 两个前端共用同一份数据：音频地址 + 标题 + 描述 + 时长，历史消息据此重放。
func PushAssistantMusic(title, summary string, duration float64, audio string) bool {
	PushMessageFunc("context", PushContextData{
		Type: "music", Audio: audio, Title: title, Summary: summary, Duration: duration,
	})
	appendCachedMessage(CachedMessage{
		ID:            newID(),
		Role:          RoleAssistant,
		Categories:    []string{"music"},
		Audio:         audio,
		MusicTitle:    title,
		MusicSummary:  summary,
		MusicDuration: duration,
		Timestamp:     time.Now().UnixMilli(),
	})
	persistConversation()
	return true
}

// pushAssistantImage 推送助手图片并写入近期记忆
func pushAssistantImage(images []string, isSticker bool) []string {
	canonical := RegisterPainterImages(images)
	PushMessageFunc("image", PushImageData{Type: "image", Images: canonical, Sticker: isSticker})
	for _, image := range canonical {
		if image == "" {
			continue
		}
		src := toDisplaySource(image)
		if src == "" || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "blob:") {
			continue
		}
		label := "图片"
		if isSticker {
			label = "表情包"
		}
		appendCachedMessage(CachedMessage{
			ID:         newID(),
			Role:       RoleAssistant,
			Categories: []string{"image"},
			ImageSrc:   src,
			ImageLabel: label,
			Timestamp:  time.Now().UnixMilli(),
		})
	}
	persistConversation()
	return canonical
}

// registerPushedImage 把待推送图片登记进句柄体系并归一化 URL
func registerPushedImage(raw string) string {
	rel := ""
	switch {
	case strings.HasPrefix(raw, fileReadPath):
		rel = strings.TrimPrefix(raw, fileReadPath)
	case strings.HasPrefix(raw, "multimedia/"), strings.HasPrefix(raw, "audios/"):
		rel = raw
	default:
		return raw
	}
	canonical, _, err := resource.RegisterPath(rel)
	if err != nil || canonical == "" || strings.HasPrefix(canonical, "@") {
		return raw
	}
	return fileReadPath + canonical
}

// appendCachedMessage 追加一条缓存消息
func appendCachedMessage(record CachedMessage) {
	cacheMu.Lock()
	cachedMessages = append(cachedMessages, record)
	if len(cachedMessages) > maxCacheMessages {
		cachedMessages = cachedMessages[len(cachedMessages)-maxCacheMessages:]
	}
	cacheMu.Unlock()
}

// persistConversation 把当前缓存写入文件
func persistConversation() {
	cacheMu.Lock()
	if len(cachedMessages) > maxCacheMessages {
		cachedMessages = cachedMessages[len(cachedMessages)-maxCacheMessages:]
	}
	text := safeJSON(cachedMessages, "[]")
	if text == lastSnapshot {
		cacheMu.Unlock()
		return
	}
	cacheMu.Unlock()
	if _, err := persistence.SaveFileEx(conversationCacheFile, persistence.SaveOptions{Overwrite: true}, strings.NewReader(text)); err != nil {
		logger.Warn(lunarCoreModule, "[对话缓存] 写入失败: %v", err)
		return
	}
	cacheMu.Lock()
	lastSnapshot = text
	cacheMu.Unlock()
}

// repairHandleImageURL 修复历史坏图片地址（句柄被当成文件路径）
func repairHandleImageURL(source string) string {
	match := handlePathRegex.FindStringSubmatch(strings.TrimSpace(source))
	if match == nil {
		return source
	}
	result, err := resource.Read(match[1], 1)
	if err == nil && !result.Lost && result.BlobPath != "" {
		return fileReadPath + result.BlobPath
	}
	return source
}

// normalizeRecord 规整缓存条目
func normalizeRecord(raw map[string]any) *CachedMessage {
	role, _ := raw["role"].(string)
	if role != RoleUser && role != RoleAssistant {
		return nil
	}
	content, _ := raw["content"].(string)
	sender, _ := raw["sender"].(string)
	if parsedSender, _ := splitUserPrefix(content); sender == "" {
		sender = parsedSender
	}
	record := &CachedMessage{
		ID:        randomIDFrom(raw["id"]),
		Role:      role,
		Content:   content,
		Sender:    sender,
		Timestamp: int64(numberArg(raw, "timestamp")),
	}
	if categories, ok := raw["categories"].([]any); ok {
		for _, item := range categories {
			if text, ok := item.(string); ok {
				record.Categories = append(record.Categories, text)
			}
		}
	}
	if len(record.Categories) == 0 {
		record.Categories = []string{"text"}
	}
	if attachments, ok := raw["attachments"].([]any); ok {
		for _, item := range attachments {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			record.Attachments = append(record.Attachments, CachedAttachment{
				Type:  stringArg(entry, "type"),
				Src:   stringArg(entry, "src"),
				Label: stringArg(entry, "label"),
			})
		}
	}
	record.ImageSrc, _ = raw["imageSrc"].(string)
	record.ImageLabel, _ = raw["imageLabel"].(string)
	record.Audio, _ = raw["audio"].(string)
	record.MusicTitle, _ = raw["musicTitle"].(string)
	record.MusicSummary, _ = raw["musicSummary"].(string)
	record.MusicDuration = numberArg(raw, "musicDuration")
	record.FileRef, _ = raw["fileRef"].(string)
	record.FileLabel, _ = raw["fileLabel"].(string)
	if record.Timestamp == 0 {
		record.Timestamp = time.Now().UnixMilli()
	}
	return record
}

func randomIDFrom(value any) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return newID()
}

// cachedToContext 把一条缓存消息还原成对话者上下文消息
func cachedToContext(record CachedMessage) *PostMessage {
	if record.Role == RoleUser {
		sender, body := splitUserPrefix(record.Content)
		if sender == "" {
			sender = strings.TrimSpace(record.Sender)
		}
		text := record.Content
		prefix := ""
		if sender != "" && body == text {
			prefix = "[用户: " + sender + "]: "
		}
		items := make([]ContentItem, 0)
		if text != "" {
			items = append(items, ContentItem{Type: ContentText, Text: prefix + text})
		} else if sender != "" {
			items = append(items, ContentItem{Type: ContentText, Text: "[用户: " + sender + "]:"})
		}
		for _, attachment := range record.Attachments {
			switch attachment.Type {
			case "image":
				items = append(items, ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{Type: mediaobject.TypeImageStatic, URL: attachment.Src}})
			case "video":
				// 历史缓存重放：视频只登记远程句柄，不重新下载落盘
				if handleText := videoHandleText(attachment.Src, channelOrigin, true); handleText != "" {
					items = append(items, ContentItem{Type: ContentText, Text: handleText})
				}
			case "audio":
				if attachment.Handle != "" || attachment.Text != "" {
					items = append(items, ContentItem{
						Type:        ContentAudioHandle,
						AudioHandle: &AudioHandlePayload{Handle: attachment.Handle, Text: attachment.Text},
					})
				} else if attachment.Src != "" {
					if handleText := audioHandleText(attachment.Src, mediaName(attachment.Src, ".wav"), channelOrigin); handleText != "" {
						items = append(items, ContentItem{Type: ContentText, Text: handleText})
					}
				}
			}
		}
		if len(items) == 0 {
			if text != "" {
				return &PostMessage{Role: RoleUser, Content: text}
			}
			return nil
		}
		// 历史重放与首次进入共用同一清洗入口；重放模式只登记视频句柄，不重新下载落盘
		converted := convertMessageResources(PostMessage{Role: RoleUser, Content: items})
		if convertedItems := asItems(converted.Content); len(convertedItems) == 1 && convertedItems[0].Type == ContentText {
			return &PostMessage{Role: RoleUser, Content: convertedItems[0].Text}
		}
		return &PostMessage{Role: RoleUser, Content: converted.Content}
	}
	for _, category := range record.Categories {
		if category == "image" || category == "action" || category == "music" {
			return nil
		}
	}
	text := strings.TrimSpace(record.Content)
	if text == "" {
		return nil
	}
	return &PostMessage{Role: RoleAssistant, Content: text}
}

// initConversationCache 读取缓存文件并载入对话者上下文
func initConversationCache(loadContext func([]PostMessage)) {
	file, _, _, err := persistence.ReadFile(conversationCacheFile)
	if err != nil {
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil || len(raw) == 0 {
		return
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		logger.Warn(lunarCoreModule, "[对话缓存] 载入失败: %v", err)
		return
	}
	records := make([]CachedMessage, 0, len(rows))
	// 历史文件里可能残留愿望到期触发的内部提醒：它们不属于用户对话，载入时剔除并回写
	removedSchedule := false
	for _, row := range rows {
		record := normalizeRecord(row)
		if record == nil {
			continue
		}
		if record.Role == RoleUser && strings.HasPrefix(strings.TrimSpace(record.Content), wishTriggerPrefix) {
			removedSchedule = true
			continue
		}
		records = append(records, *record)
	}
	if len(records) > maxCacheMessages {
		records = records[len(records)-maxCacheMessages:]
	}
	migrated := removedSchedule
	for i := range records {
		if records[i].ImageSrc != "" {
			fixed := repairHandleImageURL(records[i].ImageSrc)
			if fixed != records[i].ImageSrc {
				records[i].ImageSrc = fixed
				migrated = true
			}
		}
		for j := range records[i].Attachments {
			if records[i].Attachments[j].Type != "image" {
				continue
			}
			fixed := repairHandleImageURL(records[i].Attachments[j].Src)
			if fixed != records[i].Attachments[j].Src {
				records[i].Attachments[j].Src = fixed
				migrated = true
			}
		}
		if records[i].Role == RoleAssistant && strings.HasPrefix(records[i].ImageSrc, "data:image/") {
			if saved := saveBase64Media(records[i].ImageSrc, "image/jpeg", "image"); saved != "" {
				records[i].ImageSrc = saved
				migrated = true
			}
		}
	}
	cacheMu.Lock()
	cachedMessages = records
	if migrated {
		lastSnapshot = ""
	} else {
		lastSnapshot = safeJSON(records, "[]")
	}
	cacheMu.Unlock()
	if migrated {
		persistConversation()
	}
	context := make([]PostMessage, 0, len(records))
	for _, record := range records {
		if message := cachedToContext(record); message != nil {
			context = append(context, *message)
		}
	}
	if len(context) > 0 {
		loadContext(context)
	}
	logger.Info(lunarCoreModule, "[对话缓存] 已载入 %d 条近期记忆", len(context))
}
