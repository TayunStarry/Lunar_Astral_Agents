package DialogueBridging

// outbound.go 主动投递：群/好友目录与文本、文件发送。
//
// 与被动回复分离：主动投递不解析 [对 xxx 说]、不引用原消息，也不做触发判定；
// 由上层智能体明确指定目标与内容后直接调用。图片沿用既有 Send*ImageMessage
// （走 imageSegments 的压缩与 URL 归一化），文件走 NapCat 的上传接口。

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ==================== 目标目录 ====================

// QQTargets 读取群与好友目录（30 秒缓存）。
//
// 桥接器未连接时立即失败；群/好友接口允许部分成功（例如权限只开了群列表），
// 两个都失败才返回错误。返回的切片是缓存副本，调用方可安全读取。
func QQTargets() ([]QQGroup, []QQFriend, error) {
	if GetBridgeState() != BridgeConnected {
		return nil, nil, fmt.Errorf("QQ 桥接器未连接")
	}

	qqDirectoryMu.Lock()
	defer qqDirectoryMu.Unlock()
	if !qqDirectoryAt.IsZero() && time.Since(qqDirectoryAt) < qqDirectoryTTL {
		return append([]QQGroup(nil), qqDirectoryGroups...), append([]QQFriend(nil), qqDirectoryFriends...), qqDirectoryErr
	}

	groups, groupErr := fetchQQGroups()
	friends, friendErr := fetchQQFriends()
	switch {
	case groupErr != nil && friendErr != nil:
		qqDirectoryErr = fmt.Errorf("读取 QQ 群/好友列表失败（群: %v；好友: %v）", groupErr, friendErr)
	case groupErr != nil:
		qqDirectoryErr = fmt.Errorf("读取 QQ 群列表失败: %v", groupErr)
	case friendErr != nil:
		qqDirectoryErr = fmt.Errorf("读取 QQ 好友列表失败: %v", friendErr)
	default:
		qqDirectoryErr = nil
	}
	qqDirectoryGroups = groups
	qqDirectoryFriends = friends
	qqDirectoryAt = time.Now()
	return append([]QQGroup(nil), groups...), append([]QQFriend(nil), friends...), qqDirectoryErr
}

// BridgeReady 主动投递是否可用：连接成功 + 至少能解析出一个群/好友。
// 群列表或好友列表单独失败时，只要另一侧能解析出目标就算可用。
// 目录接口本身要求已登录，因此不再额外调用 get_login_info（避免每轮多一次接口）。
func BridgeReady() bool {
	if GetBridgeState() != BridgeConnected {
		return false
	}
	groups, friends, _ := QQTargets()
	return len(groups)+len(friends) > 0
}

// fetchQQGroups 读取群列表
func fetchQQGroups() ([]QQGroup, error) {
	resp, err := callNapcatAPI("/get_group_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var groups []QQGroup
	if err := json.Unmarshal(resp.Data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// fetchQQFriends 读取好友列表
func fetchQQFriends() ([]QQFriend, error) {
	resp, err := callNapcatAPI("/get_friend_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var friends []QQFriend
	if err := json.Unmarshal(resp.Data, &friends); err != nil {
		return nil, err
	}
	return friends, nil
}

// ==================== 主动发送 ====================

// SendGroupPlainTextMessage 主动发送群文本：原样投递，不解析 @、不引用消息。
func SendGroupPlainTextMessage(groupID int64, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return postMessage("/send_group_msg", map[string]any{"group_id": groupID}, textSegment(content))
}

// SendPrivateFile 主动发送私聊文件（file 为本地绝对路径）
func SendPrivateFile(userID int64, file, name string) error {
	return uploadQQFile("/upload_private_file", map[string]any{"user_id": userID}, file, name)
}

// SendGroupFile 主动发送群文件（file 为本地绝对路径）
func SendGroupFile(groupID int64, file, name string) error {
	return uploadQQFile("/upload_group_file", map[string]any{"group_id": groupID}, file, name)
}

// uploadQQFile 调用 NapCat 文件上传接口；上传可能较慢，使用专用长超时客户端。
func uploadQQFile(action string, target map[string]any, file, name string) error {
	file = strings.TrimSpace(file)
	if file == "" {
		return fmt.Errorf("文件路径为空")
	}
	if info, err := os.Stat(file); err != nil {
		return fmt.Errorf("文件不可读: %w", err)
	} else if info.IsDir() {
		return fmt.Errorf("不能发送文件夹: %s", file)
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(file)
	}
	// upload_file 在接口文档里是必填项（默认 true），显式带上避免不同版本按缺失处理
	params := map[string]any{"file": file, "name": name, "upload_file": true}
	for key, value := range target {
		params[key] = value
	}
	if action == "/upload_group_file" {
		params["folder"] = "" // 传到群文件根目录
	}
	_, err := callNapcatAPIWithClient(action, params, uploadHTTPClient)
	return err
}
