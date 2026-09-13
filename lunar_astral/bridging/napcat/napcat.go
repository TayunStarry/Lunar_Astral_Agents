package napcat

// Napcat WebSocket/HTTP 客户端实现
// 接口文档: https://napcat.apifox.cn/5430207m0

import (
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

// ConnectToNapcatWebSocket 连接到 Napcat WebSocket 服务器
// 成功连接后持续读取消息，断开时返回
func ConnectToNapcatWebSocket(messageHandler func([]byte)) error {
	url := bridgeConfig.BridgingPath
	token := bridgeConfig.BridgingToken

	LoggerGeneral.SubInfo("LunarCore", "Napcat", "正在连接: %s", url)

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, headers)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close()

	// 连接成功，立即设置状态为已连接
	setBridgeState(BridgeConnected)
	LoggerGeneral.SubInfo("LunarCore", "Napcat", "成功连接到 napcat 服务器")

	// 消息解析含 API 调用（回复引用、合并转发、成员名解析）可能阻塞，
	// 用单协程顺序消费与 WS 读取循环解耦：保证消息顺序的同时不拖慢读取
	msgCh := make(chan []byte, 64)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case msg := <-msgCh:
				messageHandler(msg)
			case <-done:
				return
			}
		}
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// 连接断开，重置状态
			setBridgeState(BridgeDisconnected)
			return fmt.Errorf("读取消息失败: %v", err)
		}
		select {
		case msgCh <- message:
		case <-done:
			return fmt.Errorf("读取消息失败: 连接已关闭")
		}
	}
}

// getNapcatHTTPBaseURL 将 napcat ws 地址转换为 http 地址
func getNapcatHTTPBaseURL() string {
	return strings.Replace(bridgeConfig.BridgingPath, "ws://", "http://", 1)
}

// callNapcatAPI 统一调用 Napcat HTTP API，返回完整响应
func callNapcatAPI(action string, params map[string]interface{}) (*NapcatWSResponse, error) {
	url := getNapcatHTTPBaseURL() + action
	token := bridgeConfig.BridgingToken

	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response NapcatWSResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}
	if response.Status != "ok" {
		return nil, fmt.Errorf("%s 返回错误: %s", action, response.Wording)
	}
	return &response, nil
}

// ==================== 消息发送 ====================

// maxSendTextLen 单条 QQ 消息的文本长度上限（字符数），超过自动分片连续发送
const maxSendTextLen = 4000

// mentionPattern 出站文本中的提及标记：[对 xxx 说]（与入站 @ 的渲染格式对称）
var mentionPattern = regexp.MustCompile(`\[对\s*(.+?)\s*说\]`)

// SendPrivateTextMessage 发送私聊文本消息（超长自动分片）
func SendPrivateTextMessage(userID int64, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	for _, chunk := range splitTextChunks(content, maxSendTextLen) {
		if err := postMessage("/send_private_msg", map[string]interface{}{"user_id": userID}, textSegment(chunk)); err != nil {
			return err
		}
	}
	return nil
}

// SendGroupTextMessage 发送群文本消息：
//   - 内容中的 [对 xxx 说] 转换为真实 @ 段（按群成员名称缓存解析，可 @ 成员 / 全体成员）
//   - replyToUserID 非零时在首条消息最前追加对该成员的 @（被 @ 触发的回应礼节性 @ 回对方）
func SendGroupTextMessage(groupID int64, content string, replyToUserID int64) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	for i, chunk := range splitTextChunks(content, maxSendTextLen) {
		replyTo := int64(0)
		if i == 0 {
			replyTo = replyToUserID
		}
		if err := postMessage("/send_group_msg", map[string]interface{}{"group_id": groupID}, buildOutboundSegments(groupID, chunk, replyTo)); err != nil {
			return err
		}
	}
	return nil
}

// SendPrivateImageMessage 发送私聊图片消息（base64 数据或 http 链接）
func SendPrivateImageMessage(userID int64, images []string) error {
	if err := postMessage("/send_private_msg", map[string]interface{}{"user_id": userID}, imageSegments(images)); err != nil {
		return err
	}
	return nil
}

// SendGroupImageMessage 发送群图片消息（base64 数据或 http 链接）
func SendGroupImageMessage(groupID int64, images []string) error {
	if err := postMessage("/send_group_msg", map[string]interface{}{"group_id": groupID}, imageSegments(images)); err != nil {
		return err
	}
	return nil
}

// postMessage 组装消息段并调用发送接口
func postMessage(action string, target map[string]interface{}, segments []map[string]interface{}) error {
	params := map[string]interface{}{"message": segments}
	for key, value := range target {
		params[key] = value
	}
	_, err := callNapcatAPI(action, params)
	return err
}

// textSegment 纯文本消息段
func textSegment(text string) []map[string]interface{} {
	return []map[string]interface{}{
		{"type": "text", "data": map[string]string{"text": text}},
	}
}

// atSegment @消息段
func atSegment(qq string) map[string]interface{} {
	return map[string]interface{}{
		"type": "at",
		"data": map[string]string{"qq": qq},
	}
}

// imageSegments 将图片列表转为消息段：http 链接直接引用，base64 数据剥掉 data URI 前缀
func imageSegments(images []string) []map[string]interface{} {
	segments := make([]map[string]interface{}, 0, len(images))
	for _, img := range images {
		if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
			segments = append(segments, map[string]interface{}{
				"type": "image",
				"data": map[string]string{"file": img},
			})
			continue
		}
		segments = append(segments, map[string]interface{}{
			"type": "image",
			"data": map[string]string{"file": "base64://" + normalizeBase64Image(img)},
		})
	}
	return segments
}

// buildOutboundSegments 将回应文本组装为消息段：
// 文本中的 [对 xxx 说] 转换为真实 @ 段，无法解析的提及保留原文；
// replyToUserID 非零时在最前追加 @，与其重复的提及标记直接吞掉避免双重 @
func buildOutboundSegments(groupID int64, text string, replyToUserID int64) []map[string]interface{} {
	segments := make([]map[string]interface{}, 0, 4)
	replyToQQ := ""
	if replyToUserID != 0 {
		replyToQQ = strconv.FormatInt(replyToUserID, 10)
		segments = append(segments, atSegment(replyToQQ))
	}

	lastIndex := 0
	for _, loc := range mentionPattern.FindAllStringSubmatchIndex(text, -1) {
		name := strings.TrimSpace(text[loc[2]:loc[3]])
		qq := lookupAtTarget(groupID, name)
		if qq == "" {
			// 无法解析为真实成员的提及保留原文
			continue
		}
		if replyToQQ != "" && qq == replyToQQ {
			// 前置回应 @ 已覆盖该目标，吞掉标记避免重复 @
			lastIndex = loc[1]
			continue
		}
		if loc[0] > lastIndex {
			segments = append(segments, map[string]interface{}{
				"type": "text",
				"data": map[string]string{"text": text[lastIndex:loc[0]]},
			})
		}
		segments = append(segments, atSegment(qq))
		lastIndex = loc[1]
	}
	if rest := text[lastIndex:]; rest != "" {
		segments = append(segments, map[string]interface{}{
			"type": "text",
			"data": map[string]string{"text": rest},
		})
	}
	// 回应 @ 与正文之间补一个空格，避免 QQ 端渲染成 "@某某正文"
	if replyToQQ != "" {
		for _, seg := range segments {
			if seg["type"] != "text" {
				continue
			}
			if data, ok := seg["data"].(map[string]string); ok && data["text"] != "" &&
				!strings.HasPrefix(data["text"], " ") && !strings.HasPrefix(data["text"], "\n") {
				data["text"] = " " + data["text"]
			}
			break
		}
	}
	return segments
}

// lookupAtTarget 将提及名称解析为可 @ 的目标（QQ号字符串），无法识别返回 ""
func lookupAtTarget(groupID int64, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if name == "全体成员" || name == "所有人" {
		return "all"
	}
	if name == "我" {
		// 指代月华自身，无需 @
		return ""
	}
	groupMutex.Lock()
	defer groupMutex.Unlock()
	for userID, memberName := range memberNameCache[groupID] {
		if memberName == name {
			return strconv.FormatInt(userID, 10)
		}
	}
	return ""
}

// splitTextChunks 按字符将文本切分为不超过 maxLen 的片段，优先在窗口内的换行处断开
func splitTextChunks(text string, maxLen int) []string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}
	var chunks []string
	for len(runes) > maxLen {
		cut := maxLen
		for i := maxLen - 1; i > maxLen/2; i-- {
			if runes[i] == '\n' {
				cut = i + 1
				break
			}
		}
		chunks = append(chunks, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

// normalizeBase64Image 规整图片数据：剥掉 "data:image/xxx;base64," 前缀
// NapCat 的 base64:// 仅接受纯 base64，而本系统的图片数据统一携带 data URI 前缀
func normalizeBase64Image(img string) string {
	if idx := strings.Index(img, "base64,"); idx >= 0 {
		return img[idx+len("base64,"):]
	}
	return img
}

// ==================== 消息获取与名称解析 ====================

// getMessageDetail 通过 get_msg API 获取完整消息（发送者 + 消息段），用于回复引用还原
func getMessageDetail(messageID string) (*MessageDetail, error) {
	resp, err := callNapcatAPI("/get_msg", map[string]interface{}{"message_id": messageID})
	if err != nil {
		return nil, err
	}

	var detail MessageDetail
	if err := json.Unmarshal(resp.Data, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// getFileContent 通过 get_file 接口下载文件，返回原始字节
// fileID 优先，file 作为兜底标识
func getFileContent(fileID, file string) ([]byte, error) {
	params := map[string]interface{}{}
	if fileID != "" {
		params["file_id"] = fileID
	} else if file != "" {
		params["file"] = file
	} else {
		return nil, fmt.Errorf("文件缺少 file_id / file 标识")
	}

	resp, err := callNapcatAPI("/get_file", params)
	if err != nil {
		return nil, err
	}

	var data GetFileResponse
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, err
	}
	if data.Base64 != "" {
		return base64.StdEncoding.DecodeString(data.Base64)
	}
	if data.URL != "" {
		return downloadBytes(data.URL)
	}
	if data.File != "" {
		if strings.HasPrefix(data.File, "http://") || strings.HasPrefix(data.File, "https://") {
			return downloadBytes(data.File)
		}
		return os.ReadFile(data.File)
	}
	return nil, fmt.Errorf("get_file 未返回可用的文件内容")
}

// getImageContent 通过 get_image 接口下载图片，失败时退化为 get_file
func getImageContent(file string) ([]byte, error) {
	if file == "" {
		return nil, fmt.Errorf("图片缺少 file 标识")
	}

	if resp, err := callNapcatAPI("/get_image", map[string]interface{}{"file": file}); err == nil {
		var data GetFileResponse
		if json.Unmarshal(resp.Data, &data) == nil {
			if data.Base64 != "" {
				return base64.StdEncoding.DecodeString(data.Base64)
			}
			if data.URL != "" {
				return downloadBytes(data.URL)
			}
			if data.File != "" {
				if strings.HasPrefix(data.File, "http://") || strings.HasPrefix(data.File, "https://") {
					return downloadBytes(data.File)
				}
				return os.ReadFile(data.File)
			}
		}
	}

	// get_image 失败，退化到 get_file
	return getFileContent("", file)
}

// getForwardMessageContent 通过 get_forward_msg 接口展开合并转发消息
func getForwardMessageContent(id string) ([]ForwardMessage, error) {
	resp, err := callNapcatAPI("/get_forward_msg", map[string]interface{}{"message_id": id})
	if err != nil {
		return nil, err
	}

	var data struct {
		Messages []ForwardMessage `json:"messages"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, err
	}
	return data.Messages, nil
}

// getStrangerNickname 通过 get_stranger_info 接口查询用户昵称
func getStrangerNickname(userID int64) (string, error) {
	resp, err := callNapcatAPI("/get_stranger_info", map[string]interface{}{"user_id": userID})
	if err != nil {
		return "", err
	}

	var data struct {
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}
	return data.Nickname, nil
}

// getGroupName 通过 get_group_info 接口查询群名称
func getGroupName(groupID int64) (string, error) {
	resp, err := callNapcatAPI("/get_group_info", map[string]interface{}{"group_id": groupID})
	if err != nil {
		return "", err
	}

	var data struct {
		GroupName string `json:"group_name"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}
	return data.GroupName, nil
}

// getGroupMemberInfo 通过 get_group_member_info 接口查询单个群成员信息
func getGroupMemberInfo(groupID, userID int64) (*GroupMemberInfo, error) {
	resp, err := callNapcatAPI("/get_group_member_info", map[string]interface{}{
		"group_id": groupID,
		"user_id":  userID,
	})
	if err != nil {
		return nil, err
	}

	var info GroupMemberInfo
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// getGroupMemberList 通过 get_group_member_list 接口拉取全量群成员列表
func getGroupMemberList(groupID int64) ([]GroupMemberInfo, error) {
	resp, err := callNapcatAPI("/get_group_member_list", map[string]interface{}{"group_id": groupID})
	if err != nil {
		return nil, err
	}

	var list []GroupMemberInfo
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// getVideoSource 通过 get_file 接口获取视频的可访问地址（URL 或本地文件路径）
func getVideoSource(file string) (string, error) {
	resp, err := callNapcatAPI("/get_file", map[string]interface{}{"file": file})
	if err != nil {
		return "", err
	}

	var data GetFileResponse
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return "", err
	}
	if data.URL != "" {
		return data.URL, nil
	}
	if data.File != "" {
		return data.File, nil
	}
	return "", fmt.Errorf("get_file 未返回可用的视频地址")
}

// downloadBytes 下载 http(s) 资源并返回字节
func downloadBytes(rawURL string) ([]byte, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("下载地址为空")
	}
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载失败，状态码 %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ==================== 成员名称缓存与 @ 解析 ====================

// cacheMemberName 记录群成员显示名（群名片优先），供双向 @ 解析
func cacheMemberName(groupID, userID int64, name string) {
	if groupID == 0 || userID == 0 || name == "" {
		return
	}
	groupMutex.Lock()
	defer groupMutex.Unlock()
	pool := memberNameCache[groupID]
	if pool == nil {
		pool = make(map[int64]string)
		memberNameCache[groupID] = pool
	}
	pool[userID] = name
}

// lookupMemberName 从缓存查询群成员显示名
func lookupMemberName(groupID, userID int64) string {
	groupMutex.Lock()
	defer groupMutex.Unlock()
	return memberNameCache[groupID][userID]
}

// warmGroupMembers 首次遇到某群消息时异步预取全量成员列表填充名称缓存
// （每群每进程一次；发言者与 @ 目标的名称会随消息持续增量补充）
func warmGroupMembers(groupID int64) {
	groupMutex.Lock()
	already := memberListWarmed[groupID]
	memberListWarmed[groupID] = true
	groupMutex.Unlock()
	if already {
		return
	}

	go func() {
		members, err := getGroupMemberList(groupID)
		if err != nil {
			// 失败时重置标记，下次消息重新尝试
			groupMutex.Lock()
			memberListWarmed[groupID] = false
			groupMutex.Unlock()
			return
		}
		for _, member := range members {
			name := member.Card
			if name == "" {
				name = member.Nickname
			}
			cacheMemberName(groupID, member.UserID, name)
		}
		LoggerGeneral.SubInfo("LunarCore", "Napcat", "群 %d 成员列表预热完成: %d 人", groupID, len(members))
	}()
}

// resolveGroupMemberName 查询群成员显示名（群名片优先）：缓存 → 接口，查询结果回填缓存
func resolveGroupMemberName(groupID, userID int64) string {
	if name := lookupMemberName(groupID, userID); name != "" {
		return name
	}
	info, err := getGroupMemberInfo(groupID, userID)
	if err != nil || info == nil {
		return ""
	}
	name := info.Card
	if name == "" {
		name = info.Nickname
	}
	if name != "" {
		cacheMemberName(groupID, userID, name)
	}
	return name
}

// resolveBotInfo 懒加载当前登录账号信息（get_login_info），失败时下次调用重试
func resolveBotInfo() (int64, string) {
	botInfoMutex.Lock()
	defer botInfoMutex.Unlock()
	if !botInfoLoaded {
		resp, err := callNapcatAPI("/get_login_info", map[string]interface{}{})
		if err == nil {
			var data struct {
				UserID   int64  `json:"user_id"`
				Nickname string `json:"nickname"`
			}
			if json.Unmarshal(resp.Data, &data) == nil && data.UserID != 0 {
				botUserID = data.UserID
				botNickname = data.Nickname
				botInfoLoaded = true
				LoggerGeneral.SubInfo("LunarCore", "Napcat", "当前账号: %s(%d)", botNickname, botUserID)
			}
		}
	}
	return botUserID, botNickname
}

// resolveAtDisplay 解析 @ 目标的可读名称：
// all → 全体成员；自己 → 我；群成员 → 群名片/昵称；兜底陌生人昵称，最终退回 QQ 号
func resolveAtDisplay(groupID int64, qq string) string {
	qq = strings.TrimSpace(qq)
	if qq == "" {
		return "未知用户"
	}
	if qq == "all" {
		return "全体成员"
	}
	userID, err := strconv.ParseInt(qq, 10, 64)
	if err != nil {
		return qq
	}
	if botID, _ := resolveBotInfo(); botID != 0 && userID == botID {
		return "我"
	}
	if groupID != 0 {
		if name := resolveGroupMemberName(groupID, userID); name != "" {
			return name
		}
	}
	if name, err := getStrangerNickname(userID); err == nil && name != "" {
		return name
	}
	return qq
}
