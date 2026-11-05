package napcat

// fetch.go 消息详情、文件/图片/合并转发内容与名称查询接口。

import (
	"encoding/json"
	"fmt"
)

// GetMessageDetail 通过 get_msg API 获取完整消息（发送者 + 消息段），用于回复引用还原。
func GetMessageDetail(messageID string) (*MessageDetail, error) {
	return Default().getMessageDetail(messageID)
}

// GetFileContent 通过 get_file 接口下载文件，返回原始字节。
func GetFileContent(fileID, file string) ([]byte, error) {
	return Default().getFileContent(fileID, file)
}

// GetImageContent 通过 get_image 接口下载图片，失败时退化为 get_file。
func GetImageContent(fileID, file string) ([]byte, error) {
	return Default().getImageContent(fileID, file)
}

// GetForwardMessageContent 通过 get_forward_msg 接口展开合并转发消息。
func GetForwardMessageContent(id string) ([]ForwardMessage, error) {
	return Default().getForwardMessageContent(id)
}

// GetStrangerNickname 通过 get_stranger_info 接口查询用户昵称。
func GetStrangerNickname(userID int64) (string, error) {
	return Default().getStrangerNickname(userID)
}

// GetGroupName 通过 get_group_info 接口查询群名称。
func GetGroupName(groupID int64) (string, error) {
	return Default().getGroupName(groupID)
}

// getMessageDetail 通过 get_msg API 获取完整消息（发送者 + 消息段），用于回复引用还原
func (c *Client) getMessageDetail(messageID string) (*MessageDetail, error) {
	resp, err := c.callAPI("/get_msg", map[string]any{"message_id": messageID})
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
func (c *Client) getFileContent(fileID, file string) ([]byte, error) {
	params := map[string]any{}
	if fileID != "" {
		params["file_id"] = fileID
	} else if file != "" {
		params["file"] = file
	} else {
		return nil, fmt.Errorf("文件缺少 file_id / file 标识")
	}

	resp, err := c.callAPI("/get_file", params)
	if err != nil {
		return nil, err
	}
	return decodeFileResponse(resp.Data)
}

// getImageContent 通过 get_image 接口下载图片，失败时退化为 get_file
//
// file_id 与 file 都可能单独出现（NapCat 不同版本下发的字段不同），
// 因此两个标识都要尝试；两个都为空时直接失败，避免拿空参数打接口。
func (c *Client) getImageContent(fileID, file string) ([]byte, error) {
	identifiers := make([]map[string]any, 0, 2)
	if fileID != "" {
		identifiers = append(identifiers, map[string]any{"file_id": fileID})
	}
	if file != "" {
		identifiers = append(identifiers, map[string]any{"file": file})
	}
	if len(identifiers) == 0 {
		return nil, fmt.Errorf("图片缺少 file_id / file 标识")
	}

	var lastErr error
	for _, params := range identifiers {
		resp, err := c.callAPI("/get_image", params)
		if err != nil {
			lastErr = err
			continue
		}
		data, decodeErr := decodeFileResponse(resp.Data)
		if decodeErr == nil && len(data) > 0 {
			return data, nil
		}
		lastErr = decodeErr
	}

	// get_image 全部失败，退化到 get_file（file_id 优先）
	if data, err := c.getFileContent(fileID, file); err == nil && len(data) > 0 {
		return data, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("get_image 未返回可用的图片内容")
	}
	return nil, lastErr
}

// decodeFileResponse 从 get_file / get_image 的 data 中取出文件字节
// 依次尝试 base64 → url → path → file 字段（不同 NapCat 版本填充的字段不同）
func decodeFileResponse(raw json.RawMessage) ([]byte, error) {
	var data GetFileResponse
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if data.Base64 != "" {
		return decodeBase64Payload(data.Base64)
	}
	for _, candidate := range []string{data.URL, data.Path, data.File} {
		if candidate == "" {
			continue
		}
		if bytes, err := fetchFileBytes(candidate); err == nil && len(bytes) > 0 {
			return bytes, nil
		}
	}
	return nil, fmt.Errorf("接口未返回可用的文件内容")
}

// getForwardMessageContent 通过 get_forward_msg 接口展开合并转发消息
func (c *Client) getForwardMessageContent(id string) ([]ForwardMessage, error) {
	resp, err := c.callAPI("/get_forward_msg", map[string]any{"message_id": id})
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
func (c *Client) getStrangerNickname(userID int64) (string, error) {
	resp, err := c.callAPI("/get_stranger_info", map[string]any{"user_id": userID})
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
func (c *Client) getGroupName(groupID int64) (string, error) {
	resp, err := c.callAPI("/get_group_info", map[string]any{"group_id": groupID})
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
