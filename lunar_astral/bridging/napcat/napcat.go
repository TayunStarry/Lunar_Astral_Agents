package napcat

// Napcat WebSocket/HTTP 客户端实现

import (
	"LunarSubsystem/LoggerGeneral"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// 连接断开，重置状态
			setBridgeState(BridgeDisconnected)
			return fmt.Errorf("读取消息失败: %v", err)
		}
		messageHandler(message)
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

// SendPrivateTextMessage 发送私聊文本消息
func SendPrivateTextMessage(userID int64, content string) error {
	message := []map[string]interface{}{
		{
			"type": "text",
			"data": map[string]string{"text": content},
		},
	}
	_, err := callNapcatAPI("/send_private_msg", map[string]interface{}{
		"user_id": userID,
		"message": message,
	})
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

// SendPrivateImageMessage 发送私聊图片消息（base64 编码）
func SendPrivateImageMessage(userID int64, images []string) error {
	message := make([]map[string]interface{}, 0, len(images))
	for _, img := range images {
		message = append(message, map[string]interface{}{
			"type": "image",
			"data": map[string]string{
				"file": "base64://" + normalizeBase64Image(img),
			},
		})
	}
	_, err := callNapcatAPI("/send_private_msg", map[string]interface{}{
		"user_id": userID,
		"message": message,
	})
	return err
}

// SendGroupTextMessage 发送群文本消息
func SendGroupTextMessage(groupID int64, content string) error {
	message := []map[string]interface{}{
		{
			"type": "text",
			"data": map[string]string{"text": content},
		},
	}
	_, err := callNapcatAPI("/send_group_msg", map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	})
	return err
}

// SendGroupImageMessage 发送群图片消息（base64 编码）
func SendGroupImageMessage(groupID int64, images []string) error {
	message := make([]map[string]interface{}, 0, len(images))
	for _, img := range images {
		message = append(message, map[string]interface{}{
			"type": "image",
			"data": map[string]string{
				"file": "base64://" + normalizeBase64Image(img),
			},
		})
	}
	_, err := callNapcatAPI("/send_group_msg", map[string]interface{}{
		"group_id": groupID,
		"message":  message,
	})
	return err
}

// GetMessageContent 通过 get_msg API 获取单条消息的文本内容
func GetMessageContent(messageID string) (string, error) {
	resp, err := callNapcatAPI("/get_msg", map[string]interface{}{"message_id": messageID})
	if err != nil {
		return "", err
	}

	var msgData struct {
		Message []MessageSegment `json:"message"`
	}
	if err := json.Unmarshal(resp.Data, &msgData); err != nil {
		return "", err
	}

	var content string
	for _, seg := range msgData.Message {
		if seg.Type != "text" {
			continue
		}
		var textData TextData
		if json.Unmarshal(seg.Data, &textData) == nil {
			content += textData.Text
		}
	}
	return content, nil
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
