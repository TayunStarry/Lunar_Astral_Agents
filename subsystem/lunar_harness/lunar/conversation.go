package lunar

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

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/logger"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	persistence "LunarSubsystem/LunarHarness/engine/persistence"
	StarLTP "LunarSubsystem/LunarHarness/engine/plugin"
)

var (
	userPrefixRegex = regexp.MustCompile(`^\[用户:\s*([^\]]*)\]\s*[:：]\s*`)
	fileRefRegex    = regexp.MustCompile(`@[DIVAT]\.\d{3,}`)
	noticeRegex     = regexp.MustCompile(`^\[已收录\]\s*(@[DIVAT]\.\d{3,})\s*(.*)$`)
	handlePathRegex = regexp.MustCompile(`^/file/read/(@[DIVAT]\.\d{3,})/?$`)
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

// splitUserPrefix 拆出 `[用户: 名字]: ` 前缀
func splitUserPrefix(text string) (sender, body string) {
	match := userPrefixRegex.FindStringSubmatch(text)
	if match == nil {
		return "", text
	}
	return strings.TrimSpace(match[1]), text[len(match[0]):]
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

// toVisionSource 转成思维链视觉读取地址（非本地图片返回空串）
func toVisionSource(source string) string {
	raw := strings.TrimSpace(source)
	if raw == "" || strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "blob:") {
		return ""
	}
	path := pathPartOf(raw)
	if index := strings.Index(path, visionReadPath); index >= 0 {
		return config.LocalBaseURL() + path[index:]
	}
	if index := strings.Index(path, fileReadPath); index >= 0 {
		rel := path[index+len(fileReadPath):]
		if !strings.HasPrefix(rel, "multimedia/") {
			return ""
		}
		return config.LocalBaseURL() + visionReadPath + rel
	}
	if strings.HasPrefix(path, "multimedia/") {
		return config.LocalBaseURL() + visionReadPath + path
	}
	return ""
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
	if kind != "voice" && !strings.HasPrefix(contentType, "image/") {
		contentType = "image/png"
	}
	extMap := map[string]string{
		"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif", "image/bmp": "bmp",
		"audio/wav": "wav", "audio/x-wav": "wav", "audio/mpeg": "mp3", "audio/mp3": "mp3", "audio/flac": "flac", "audio/ogg": "ogg",
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
		if parsedBody == "" || parsedBody == "[视频]" || parsedBody == "[语音]" {
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
			case ContentVideoURL:
				if item.VideoURL != nil {
					pushAttachment("video", item.VideoURL.URL, "")
					hasImage = true
				}
			case ContentInputAudio:
				if item.InputAudio != nil {
					format := item.InputAudio.Format
					if format == "" {
						format = "wav"
					}
					src := saveBase64Media(item.InputAudio.Data, "audio/"+format, "voice")
					if src != "" {
						attachments = append(attachments, CachedAttachment{Type: "audio", Src: src, Label: "语音"})
						hasVoice = true
					}
				}
			case ContentAudioURL:
				if item.AudioURL != nil {
					pushAttachment("audio", item.AudioURL.URL, "语音")
					hasVoice = true
				}
			}
		}
	}

	text := strings.TrimSpace(strings.Join(texts, "\n"))
	if text == "" && len(attachments) == 0 {
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

// recordAssistantContext 记录一条助手文本/语音/乐谱/动作消息
func recordAssistantContext(kind, content, audio string) {
	if kind == "music_audio" {
		return
	}
	record := CachedMessage{
		ID:         newID(),
		Role:       RoleAssistant,
		Categories: []string{"text"},
		Content:    content,
		Timestamp:  time.Now().UnixMilli(),
	}
	switch kind {
	case "music":
		record.Categories = []string{"music"}
		record.Content = ""
		record.ABCNotation = content
	case "action":
		record.Categories = []string{"action"}
		record.ActionType = kind
	case "text":
		if audio != "" {
			record.Categories = []string{"text", "voice"}
			record.Audio = audio
		}
		if match := noticeRegex.FindStringSubmatch(strings.TrimSpace(record.Content)); match != nil {
			record.Categories = []string{"text", "file"}
			record.FileRef = match[1]
			record.FileLabel = match[2]
			if record.FileLabel == "" {
				record.FileLabel = match[1]
			}
		}
	default:
		record.Categories = []string{kind}
	}
	appendCachedMessage(record)
}

// pushContext 推送上下文数据（含插件消息发送前事件）
func pushContext(kind, content, audio string, auto bool) bool {
	items := dispatchBeforeSendingMessage([]PushContextData{{Type: kind, Content: content, Audio: audio, Auto: auto}})
	if items == nil {
		return false
	}
	for _, item := range items {
		PushMessageFunc("context", item)
	}
	return true
}

// dispatchBeforeSendingMessage 消息发送前事件：插件可改写/追加/取消
func dispatchBeforeSendingMessage(items []PushContextData) []PushContextData {
	if len(items) == 0 {
		return items
	}
	result := StarLTP.Emit(StarLTP.EventBeforeSendingMessage, map[string]any{"messages": items}, "")
	feedback, ok := result.Return.(map[string]any)
	if !result.Returned || !ok {
		return items
	}
	if cancel, _ := feedback["cancel"].(bool); cancel {
		return nil
	}
	raw, has := feedback["messages"]
	if !has {
		return items
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return items
	}
	var out []PushContextData
	if err := json.Unmarshal(data, &out); err != nil {
		return items
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// pushAssistantContext 推送助手上下文并写入缓存
func pushAssistantContext(kind, content, audio string, auto bool) bool {
	ok := pushContext(kind, content, audio, auto)
	recordAssistantContext(kind, content, audio)
	persistConversation()
	return ok
}

// pushAssistantImage 推送助手图片并写入近期记忆
func pushAssistantImage(images []string, isSticker bool) []string {
	canonical := make([]string, 0, len(images))
	for _, image := range images {
		canonical = append(canonical, registerPushedImage(image))
	}
	if len(canonical) == 0 {
		canonical = images
	}
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
	canonical, _, err := persistence.RegisterPath(rel)
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
	result, err := persistence.ResourceRead(match[1], 1)
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
	record.ABCNotation, _ = raw["abcNotation"].(string)
	record.ActionType, _ = raw["actionType"].(string)
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
				vision := toVisionSource(attachment.Src)
				if vision == "" {
					continue
				}
				animated, err := isAnimatedImageSource(attachment.Src)
				if err != nil {
					items = append(items, ContentItem{Type: ContentText, Text: "[历史图片不可用：" + attachment.Label + "]"})
					continue
				}
				if animated {
					summary := strings.TrimSpace(getPromptFromKnowledgeAny(animatedSummaryKeys(attachment.Src)))
					label := "[动态图]"
					if summary != "" {
						label = "[动态图: " + truncateRunes(summary, 1200) + "]"
					}
					items = append(items, ContentItem{Type: ContentText, Text: label})
					continue
				}
				items = append(items, ContentItem{Type: ContentImageURL, ImageURL: &URLPayload{URL: vision}})
			case "video":
				display := toDisplaySource(attachment.Src)
				if strings.HasPrefix(display, fileReadPath) {
					items = append(items, ContentItem{Type: ContentVideoURL, VideoURL: &URLPayload{URL: config.LocalBaseURL() + display}})
				}
			}
		}
		if len(items) == 0 {
			if text != "" {
				return &PostMessage{Role: RoleUser, Content: text}
			}
			return nil
		}
		if len(items) == 1 && items[0].Type == ContentText {
			return &PostMessage{Role: RoleUser, Content: items[0].Text}
		}
		return &PostMessage{Role: RoleUser, Content: items}
	}
	for _, category := range record.Categories {
		if category == "image" || category == "action" {
			return nil
		}
	}
	if record.ABCNotation != "" {
		return nil
	}
	text := strings.TrimSpace(record.Content)
	if text == "" {
		return nil
	}
	return &PostMessage{Role: RoleAssistant, Content: text}
}

// animatedSummaryKeys 构造动图理解缓存候选键
func animatedSummaryKeys(source string) []string {
	raw := strings.TrimSpace(source)
	if raw == "" {
		return nil
	}
	normalize := func(value string) string { return pathPartOf(value) }
	vision := toVisionSource(raw)
	keys := make([]string, 0, 8)
	seen := map[string]bool{}
	for _, value := range []string{raw, vision, normalize(raw), normalize(vision)} {
		if value != "" && !seen[value] {
			seen[value] = true
			keys = append(keys, value)
		}
	}
	path := normalize(raw)
	if marker := strings.Index(path, "/multimedia/"); marker >= 0 {
		rel := path[marker+1:]
		for _, value := range []string{rel, fileReadPath + rel, visionReadPath + rel, "file://" + rel} {
			if !seen[value] {
				seen[value] = true
				keys = append(keys, value)
			}
		}
		base := config.LocalBaseURL()
		for _, value := range []string{base + fileReadPath + rel, base + visionReadPath + rel} {
			if !seen[value] {
				seen[value] = true
				keys = append(keys, value)
			}
		}
	}
	return keys
}

// isAnimatedImageSource 判定来源是否为动态图
func isAnimatedImageSource(source string) (bool, error) {
	cacheMu.Lock()
	if value, ok := animatedHistoryCache[source]; ok {
		cacheMu.Unlock()
		return value, nil
	}
	cacheMu.Unlock()
	data, err := resolveImageBytes(source)
	if err != nil {
		return false, err
	}
	animated := multimodal.IsAnimatedImage(data)
	cacheMu.Lock()
	animatedHistoryCache[source] = animated
	cacheMu.Unlock()
	return animated, nil
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
	for _, row := range rows {
		if record := normalizeRecord(row); record != nil {
			records = append(records, *record)
		}
	}
	if len(records) > maxCacheMessages {
		records = records[len(records)-maxCacheMessages:]
	}
	migrated := false
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

// guessImageMime 按 base64 头部猜图片 MIME
func guessImageMime(base64Text string) string {
	head := base64Text
	if len(head) > 16 {
		head = head[:16]
	}
	switch {
	case strings.HasPrefix(head, "/9j/"):
		return "image/jpeg"
	case strings.HasPrefix(head, "iVBOR"):
		return "image/png"
	case strings.HasPrefix(head, "R0lG"):
		return "image/gif"
	case strings.HasPrefix(head, "UklGR"):
		return "image/webp"
	case strings.HasPrefix(head, "Qk"):
		return "image/bmp"
	}
	return "image/png"
}
