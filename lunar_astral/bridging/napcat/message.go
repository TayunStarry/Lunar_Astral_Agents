package napcat

// 消息解析与单线程收发流程

import (
	"LunarSubsystem/LoggerGeneral"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// maxGroupPoolSize 单个群聊缓存池的最大容量
const maxGroupPoolSize = 20

// HandleNapcatMessage 处理从 Napcat 接收到的消息（私聊 / 群聊）
func HandleNapcatMessage(rawMessage []byte) {
	var napcatMsg NapcatMessage
	if err := json.Unmarshal(rawMessage, &napcatMsg); err != nil {
		LoggerGeneral.SubError("LunarCore", "Napcat", "解析消息失败: %v", err)
		return
	}

	// 忽略自身发出的消息
	if napcatMsg.SelfID != 0 && napcatMsg.UserID == napcatMsg.SelfID {
		return
	}

	// 仅处理 message 事件，忽略 notice / request / meta_event / message_sent 等
	if napcatMsg.PostType != "" && napcatMsg.PostType != "message" {
		return
	}

	switch napcatMsg.MessageType {
	case "private":
		handlePrivateMessage(napcatMsg)
	case "group":
		handleGroupMessage(napcatMsg)
	}
}

// handlePrivateMessage 处理私聊消息：即时构建请求并入队
func handlePrivateMessage(msg NapcatMessage) {
	if !isAllowedTarget(msg.UserID) {
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "忽略未授权用户 %d 的私聊消息", msg.UserID)
		return
	}

	nickname := resolveNickname(msg.UserID, msg.Sender)
	content, hasImages, videoURLs := parseMessageSegments(msg.Message)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	if rp := parseRawRedPacket(msg.Raw); rp != nil {
		content = buildRedPacketText(rp)
		hasImages = false
	}

	message := buildUserMessage("[用户: "+nickname+"]: ", content, hasImages)

	enqueueRequest(BridgeRequest{
		Target:    BridgeTarget{ID: msg.UserID, IsGroup: false},
		Messages:  []map[string]interface{}{message},
		VideoURLs: videoURLs,
	})
}

// handleGroupMessage 处理群聊消息：内置缓存池与触发机制
func handleGroupMessage(msg NapcatMessage) {
	if !isAllowedTarget(msg.GroupID) {
		return
	}

	groupName := resolveGroupName(msg.GroupID)
	memberName := resolveMemberName(msg.Sender)

	content, hasImages, videoURLs := parseMessageSegments(msg.Message)
	textContent := contentToText(content)

	// 红包感知：红包消息承载于 raw.elements[].walletElement（message 段为空）
	redPacket := parseRawRedPacket(msg.Raw)
	if redPacket != nil {
		content = buildRedPacketText(redPacket)
		hasImages = false
		textContent = contentToText(content)
	}

	entry := GroupPoolEntry{Nickname: memberName, Content: content, HasImages: hasImages, VideoURLs: videoURLs}

	atSelf := containsAtSelf(msg.Message, msg.SelfID)
	mentioned := containsAnyKeyword(textContent, bridgeConfig.BridgingGroupKeywords)
	triggerProbability := groupTriggerProbability()
	randomTrigger := triggerProbability > 0 && rand.Float64() < triggerProbability

	if atSelf || mentioned || redPacket != nil || randomTrigger {
		// 触发：将缓存池连同当前消息一并发送给月华（至多20条）
		entries := snapshotGroupPool(msg.GroupID)
		entries = append(entries, entry)
		if len(entries) > maxGroupPoolSize {
			entries = entries[len(entries)-maxGroupPoolSize:]
		}
		clearGroupPool(msg.GroupID)

		req := BridgeRequest{
			Target:    BridgeTarget{ID: msg.GroupID, IsGroup: true, GroupName: groupName},
			Messages:  buildGroupMessages(groupName, entries),
			VideoURLs: collectGroupVideoURLs(entries),
		}

		if redPacket != nil && redPacket.IsPhrase {
			// 口令红包：等待随机 0.5~3 秒再推送，让月华复读口令领取红包
			go func() {
				time.Sleep(randomRedPacketDelay())
				enqueueRequest(req)
			}()
		} else {
			enqueueRequest(req)
		}
	} else {
		// 未触发：累积到缓存池
		addToGroupPool(msg.GroupID, groupName, entry)
	}
}

// isAllowedTarget 判断用户QQ号或群号是否在允许响应的白名单中
func isAllowedTarget(id int64) bool {
	for _, allowed := range bridgeConfig.BridgingUsers {
		if allowed == id {
			return true
		}
	}
	return false
}

// groupTriggerProbability 群聊随机应答概率：未配置时默认 0.3
func groupTriggerProbability() float64 {
	if bridgeConfig.BridgingGroupTriggerProbability != nil {
		return *bridgeConfig.BridgingGroupTriggerProbability
	}
	return 0.3
}

// containsAnyKeyword 判断文本是否包含关键词列表中的任意一个
func containsAnyKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		if kw != "" && strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// parseRawRedPacket 从 raw.elements 中识别红包（walletElement），非红包返回 nil
func parseRawRedPacket(raw json.RawMessage) *RedPacketInfo {
	if len(raw) == 0 {
		return nil
	}
	var payload struct {
		Elements []struct {
			Wallet *WalletElement `json:"walletElement"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	for _, e := range payload.Elements {
		if e.Wallet == nil {
			continue
		}
		return &RedPacketInfo{
			IsRedPacket: true,
			IsPhrase:    e.Wallet.RedChannel == 32 || e.Wallet.MsgType == 6,
			Blessing:    walletBlessing(e.Wallet),
			BillNo:      e.Wallet.BillNo,
		}
	}
	return nil
}

// walletBlessing 提取红包祝福语：receiver.title 优先，notice 去掉 "[QQ红包]" 前缀兜底
func walletBlessing(w *WalletElement) string {
	if w.Receiver.Title != "" {
		return w.Receiver.Title
	}
	if notice := strings.TrimPrefix(w.Receiver.Notice, "[QQ红包]"); notice != "" {
		return notice
	}
	return w.Receiver.Content
}

// buildRedPacketText 将红包信息渲染为文本（口令红包附带复读领取指令）
func buildRedPacketText(rp *RedPacketInfo) string {
	if rp.IsPhrase {
		return fmt.Sprintf("[口令红包] 口令：『%s』，请直接复读口令领取红包 ", rp.Blessing)
	}
	return fmt.Sprintf("[红包] 祝福语: %s ", rp.Blessing)
}

// randomRedPacketDelay 领取口令红包前的随机等待时间（0.5~3 秒）
func randomRedPacketDelay() time.Duration {
	return time.Duration(500+rand.Intn(2500)) * time.Millisecond
}

// resolveNickname 解析私聊用户昵称：优先 sender.nickname，其次 card，最后通过接口查询
func resolveNickname(userID int64, sender Sender) string {
	if sender.Nickname != "" {
		return sender.Nickname
	}
	if sender.Card != "" {
		return sender.Card
	}
	if name, err := getStrangerNickname(userID); err == nil && name != "" {
		return name
	}
	return strconv.FormatInt(userID, 10)
}

// resolveMemberName 解析群聊发言用户昵称：群名片优先，昵称兜底
func resolveMemberName(sender Sender) string {
	if sender.Card != "" {
		return sender.Card
	}
	return sender.Nickname
}

// containsAtSelf 判断消息段中是否 @了当前账号
func containsAtSelf(segments []MessageSegment, selfID int64) bool {
	selfStr := strconv.FormatInt(selfID, 10)
	for _, segment := range segments {
		if segment.Type != "at" {
			continue
		}
		var atData AtData
		if json.Unmarshal(segment.Data, &atData) == nil && atData.QQ == selfStr {
			return true
		}
	}
	return false
}

// resolveGroupName 解析并缓存群名称
func resolveGroupName(groupID int64) string {
	groupMutex.Lock()
	if name, ok := groupNameCache[groupID]; ok && name != "" {
		groupMutex.Unlock()
		return name
	}
	groupMutex.Unlock()

	name, err := getGroupName(groupID)
	if err != nil || name == "" {
		name = strconv.FormatInt(groupID, 10)
	}

	groupMutex.Lock()
	groupNameCache[groupID] = name
	groupMutex.Unlock()
	return name
}

// snapshotGroupPool 获取群聊缓存池的快照
func snapshotGroupPool(groupID int64) []GroupPoolEntry {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		return nil
	}
	out := make([]GroupPoolEntry, len(pool.Entries))
	copy(out, pool.Entries)
	return out
}

// addToGroupPool 添加消息到群聊缓存池，超出容量时抛弃最老消息
func addToGroupPool(groupID int64, groupName string, entry GroupPoolEntry) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := groupPools[groupID]
	if pool == nil {
		pool = &GroupPool{GroupID: groupID, GroupName: groupName}
		groupPools[groupID] = pool
	}
	pool.Entries = append(pool.Entries, entry)
	if len(pool.Entries) > maxGroupPoolSize {
		pool.Entries = pool.Entries[len(pool.Entries)-maxGroupPoolSize:]
	}
}

// clearGroupPool 清空指定群聊的缓存池
func clearGroupPool(groupID int64) {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	delete(groupPools, groupID)
}

// buildGroupMessages 将群聊缓存池条目构建为 OpenAI 格式消息（带群聊前缀）
func buildGroupMessages(groupName string, entries []GroupPoolEntry) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		prefix := fmt.Sprintf("[群聊: %s][用户: %s]: ", groupName, e.Nickname)
		messages = append(messages, buildUserMessage(prefix, e.Content, e.HasImages))
	}
	return messages
}

// collectGroupVideoURLs 汇总群聊缓存池条目中的视频地址
func collectGroupVideoURLs(entries []GroupPoolEntry) []string {
	var urls []string
	for _, e := range entries {
		urls = append(urls, e.VideoURLs...)
	}
	return urls
}

// contentToText 从解析结果（string 或 多模态数组）中提取纯文本
func contentToText(content interface{}) string {
	if s, ok := content.(string); ok {
		return s
	}
	if arr, ok := content.([]map[string]interface{}); ok {
		var sb strings.Builder
		for _, item := range arr {
			if item["type"] == "text" {
				if t, ok := item["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// buildUserMessage 将内容与发送者前缀组装为单条 OpenAI 消息
func buildUserMessage(prefix string, content interface{}, hasImages bool) map[string]interface{} {
	if hasImages {
		arr, _ := content.([]map[string]interface{})
		withPrefix := append([]map[string]interface{}{{"type": "text", "text": prefix}}, arr...)
		return map[string]interface{}{"role": "user", "content": withPrefix}
	}
	text, _ := content.(string)
	return map[string]interface{}{"role": "user", "content": prefix + text}
}

// enqueueRequest 将请求加入队列并尝试推进
func enqueueRequest(req BridgeRequest) {
	flowMutex.Lock()
	requestQueue = append(requestQueue, req)
	flowMutex.Unlock()
	pumpNext()
}

// pumpNext 在空闲时（无待回应请求）推送队列头的请求给月华
func pumpNext() {
	flowMutex.Lock()
	defer flowMutex.Unlock()

	if awaitingResponse || len(requestQueue) == 0 {
		return
	}

	req := requestQueue[0]
	requestQueue = requestQueue[1:]
	currentTarget = req.Target
	awaitingResponse = true

	if SendMessageToAgent != nil {
		SendMessageToAgent(req.Messages)
	} else {
		awaitingResponse = false
		LoggerGeneral.SubError("LunarCore", "Napcat", "SendMessageToAgent 回调未注册，无法推送消息")
	}
	if SendVideoToAgent != nil && len(req.VideoURLs) > 0 {
		SendVideoToAgent(req.VideoURLs)
	}
}

// parseMessageSegments 解析消息段列表，返回 (内容, 是否含图片, 视频地址列表)
// 纯文本返回 string，包含图片返回 []map[string]interface{}；视频地址写入第三返回值
func parseMessageSegments(segments []MessageSegment) (interface{}, bool, []string) {
	var contentArray []map[string]interface{}
	var contentStr string
	var hasImages bool
	var videoURLs []string

	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				appendContent(&contentArray, &contentStr, textData.Text)
			}
		case "at":
			var atData AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				appendContent(&contentArray, &contentStr, "@"+atData.QQ+" ")
			}
		case "reply":
			var replyData ReplyData
			if json.Unmarshal(segment.Data, &replyData) == nil {
				replyText, err := GetMessageContent(replyData.ID)
				if err != nil || replyText == "" {
					appendContent(&contentArray, &contentStr, "[回复] ")
				} else {
					appendContent(&contentArray, &contentStr, "[回复: "+replyText+"] ")
				}
			}
		case "image":
			var imageData ImageData
			if json.Unmarshal(segment.Data, &imageData) == nil {
				imgURL := resolveImageURL(imageData)
				if imgURL != "" {
					hasImages = true
					markMultimedia(&contentArray, &contentStr)
					contentArray = append(contentArray, map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]string{"url": imgURL},
					})
				}
			}
		case "video":
			var videoData VideoData
			if json.Unmarshal(segment.Data, &videoData) == nil {
				if source := resolveVideoSource(videoData); source != "" {
					videoURLs = append(videoURLs, source)
				}
				appendContent(&contentArray, &contentStr, "[视频] ")
			}
		case "file":
			var fileData FileData
			if json.Unmarshal(segment.Data, &fileData) == nil {
				fileText, isImage, dataURI := processFileSegment(fileData)
				if isImage {
					hasImages = true
					markMultimedia(&contentArray, &contentStr)
					contentArray = append(contentArray, map[string]interface{}{
						"type":      "image_url",
						"image_url": map[string]string{"url": dataURI},
					})
				} else {
					appendContent(&contentArray, &contentStr, fileText)
				}
			}
		case "forward":
			var forwardData ForwardData
			if json.Unmarshal(segment.Data, &forwardData) == nil {
				appendContent(&contentArray, &contentStr, processForwardSegment(forwardData.ID))
			}
		default:
			// 忽略其余消息段类型
		}
	}

	if hasImages {
		return contentArray, true, videoURLs
	}
	return contentStr, false, videoURLs
}

// resolveImageURL 获取图片的可访问地址：优先使用 url，其次通过接口下载为 base64 data URI
func resolveImageURL(imageData ImageData) string {
	if imageData.URL != "" {
		return imageData.URL
	}
	if imageData.File != "" {
		if bytes, err := getImageContent(imageData.File); err == nil && len(bytes) > 0 {
			return "data:image/png;base64," + base64.StdEncoding.EncodeToString(bytes)
		}
	}
	return ""
}

// resolveVideoSource 获取视频的可访问地址（优先 url，其次通过 get_file 下载）
func resolveVideoSource(videoData VideoData) string {
	if videoData.URL != "" {
		return videoData.URL
	}
	if videoData.File != "" {
		if source, err := getVideoSource(videoData.File); err == nil && source != "" {
			return source
		}
	}
	return ""
}

// processFileSegment 处理文件消息段，返回 (文本描述, 是否图片, 图片dataURI)
func processFileSegment(fileData FileData) (string, bool, string) {
	fileName := fileData.FileName
	if fileName == "" {
		fileName = fileData.Name
	}
	if fileName == "" {
		fileName = fileData.File
	}

	// 未提供文件标识时，仅回传名称与大小信息
	if fileData.FileID == "" && fileData.File == "" {
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	data, err := getFileContent(fileData.FileID, fileData.File)
	if err != nil || len(data) == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "下载文件失败: %v", err)
		return describeFile(fileName, fileData.FileSize), false, ""
	}

	// 图片文件 → 作为多媒体内容处理
	if mime := imageMIME(fileName); mime != "" {
		return "", true, "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	}

	// 文本文件 → 打包为阅读者可识别的文件围栏块 ```fileName\n全文\n```
	// 阅读者会在思考循环前将其切片入库，并把围栏块置换为 [已导入文件 #fileName]
	if isTextFile(fileName) {
		text := strings.TrimSpace(string(data))
		if text == "" {
			return describeFile(fileName, fileData.FileSize), false, ""
		}
		return "```" + fileName + "\n" + text + "\n```", false, ""
	}

	// 其他类型 → 保留接口，仅回传文件大小与名称
	return describeFile(fileName, fileData.FileSize), false, ""
}

// describeFile 构建文件的名称与大小信息描述
func describeFile(fileName string, sizeRaw json.RawMessage) string {
	var sb strings.Builder
	sb.WriteString("[文件]")
	if fileName != "" {
		sb.WriteString(" 名称: ")
		sb.WriteString(fileName)
	}
	if size := fileSizeText(sizeRaw); size != "" {
		sb.WriteString(" 大小: ")
		sb.WriteString(size)
	}
	sb.WriteString(" ")
	return sb.String()
}

// fileSizeText 将 file_size 字段（可能为数字或字符串）解析为字符串
func fileSizeText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return s
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return ""
}

// isTextFile 通过扩展名判断是否为文本文件
func isTextFile(fileName string) bool {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), ".")) {
	case "txt", "md", "markdown", "log", "json", "xml", "csv", "tsv", "yaml", "yml",
		"ini", "cfg", "conf", "toml", "py", "go", "js", "ts", "jsx", "tsx",
		"java", "c", "cpp", "h", "hpp", "cs", "rb", "php", "sh", "bat", "ps1",
		"sql", "html", "htm", "css", "rs", "kt", "swift":
		return true
	}
	return false
}

// imageMIME 通过扩展名判断并返回图片 MIME 类型，非图片返回空字符串
func imageMIME(fileName string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(fileName), ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// processForwardSegment 展开合并转发消息，拼接为文本描述
func processForwardSegment(id string) string {
	messages, err := getForwardMessageContent(id)
	if err != nil || len(messages) == 0 {
		LoggerGeneral.SubError("LunarCore", "Napcat", "获取合并转发消息失败: %v", err)
		return "[转发消息] "
	}

	var sb strings.Builder
	sb.WriteString("[转发消息]\n")
	for _, msg := range messages {
		sender := msg.Sender.Nickname
		if sender == "" {
			sender = strconv.FormatInt(msg.Sender.UserID, 10)
		}
		segments := msg.Message
		if len(segments) == 0 {
			segments = msg.Content
		}
		sb.WriteString(sender)
		sb.WriteString(": ")
		sb.WriteString(extractSegmentText(segments))
		sb.WriteString("\n")
	}
	return sb.String()
}

// extractSegmentText 从消息段列表中提取纯文本（用于群聊缓存与合并转发子消息的摘要）
func extractSegmentText(segments []MessageSegment) string {
	var sb strings.Builder
	for _, segment := range segments {
		switch segment.Type {
		case "text":
			var textData TextData
			if json.Unmarshal(segment.Data, &textData) == nil {
				sb.WriteString(textData.Text)
			}
		case "at":
			var atData AtData
			if json.Unmarshal(segment.Data, &atData) == nil {
				sb.WriteString("@")
				sb.WriteString(atData.QQ)
				sb.WriteString(" ")
			}
		case "image":
			sb.WriteString("[图片]")
		case "video":
			sb.WriteString("[视频]")
		case "file":
			sb.WriteString("[文件]")
		case "face":
			sb.WriteString("[表情]")
		}
	}
	return sb.String()
}

// appendContent 根据当前内容格式追加文本
// 如果已经是数组格式（有图片），追加为 text 类型元素；否则追加到纯字符串
func appendContent(contentArray *[]map[string]interface{}, contentStr *string, text string) {
	if len(*contentArray) > 0 {
		*contentArray = append(*contentArray, map[string]interface{}{
			"type": "text",
			"text": text,
		})
	} else {
		*contentStr += text
	}
}

// markMultimedia 首次遇到多媒体时将已累积的纯文本迁移到数组格式
func markMultimedia(contentArray *[]map[string]interface{}, contentStr *string) {
	if len(*contentArray) == 0 && *contentStr != "" {
		*contentArray = append(*contentArray, map[string]interface{}{
			"type": "text",
			"text": *contentStr,
		})
		*contentStr = ""
	}
}
