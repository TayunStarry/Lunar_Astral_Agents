package napcat

// upload.go 主动投递：纯文本与文件上传发送。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SendGroupPlainTextMessage 主动发送群文本：原样投递，不解析 @、不引用消息。
func SendGroupPlainTextMessage(groupID int64, content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	return postMessage("/send_group_msg", map[string]any{"group_id": groupID}, textSegment(content))
}

// SendPrivateFile 主动发送私聊文件（file 为本地绝对路径）
func SendPrivateFile(userID int64, file, name string) error {
	return uploadFile("/upload_private_file", map[string]any{"user_id": userID}, file, name)
}

// SendGroupFile 主动发送群文件（file 为本地绝对路径）
func SendGroupFile(groupID int64, file, name string) error {
	return uploadFile("/upload_group_file", map[string]any{"group_id": groupID}, file, name)
}

// uploadFile 调用 NapCat 文件上传接口；上传可能较慢，使用专用长超时客户端。
func uploadFile(action string, target map[string]any, file, name string) error {
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
	client := Default()
	_, err := client.callAPIWithClient(action, params, client.uploadHTTPClient)
	if err != nil {
		// NapCat 的文件上传在「文件已落盘上传」之后还会等待 onMsgInfoListUpdate 事件回执；
		// 该事件偶尔超时（Wording: Timeout: NTEvent ... sendMsg ...），但文件其实已经发送成功。
		// 若把这类「回执超时」判为投递失败，用户会在群里明明看到文件、日志却出现失败警告。
		if isUnconfirmedUpload(err) {
			logger.Info("DialogueBridging", "文件上传已提交但未收到 NapCat 事件回执，按成功处理: %s（%v）", name, err)
			return nil
		}
		return err
	}
	return nil
}

// isUnconfirmedUpload 判断是否为 NapCat「上传已完成、仅事件回执超时」的错误。
func isUnconfirmedUpload(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	if !strings.Contains(text, "Timeout: NTEvent") {
		return false
	}
	return strings.Contains(text, "sendMsg") || strings.Contains(text, "onMsgInfoListUpdate")
}
