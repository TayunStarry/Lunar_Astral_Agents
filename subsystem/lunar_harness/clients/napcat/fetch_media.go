package napcat

// fetch_media.go 媒体/文件字节获取与成员列表接口。

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"LunarSubsystem/LunarHarness/engine/netsource"
)

// GetVideoSource 通过 get_file 接口获取视频的可访问地址（URL 或本地文件路径）。
func GetVideoSource(file string) (string, error) {
	return Default().getVideoSource(file)
}

// GetRecordSource 通过 get_record 接口把语音转码为 mp3 并返回可访问来源。
func GetRecordSource(file string) (*GetFileResponse, error) {
	resp, err := Default().callAPI("/get_record", map[string]any{"file": file, "out_format": "mp3"})
	if err != nil {
		return nil, err
	}
	var data GetFileResponse
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// FetchFileBytes 获取 napcat 返回的资源字节（data URI / http(s) / 本地路径）。
func FetchFileBytes(raw string) ([]byte, error) {
	return fetchFileBytes(raw)
}

// FileURLPath 把 file:// 地址还原为本地路径（兼容 Windows 的 file:///C:/...）。
func FileURLPath(raw string) string {
	return fileURLPath(raw)
}

// getVideoSource 通过 get_file 接口获取视频的可访问地址
func (c *Client) getVideoSource(file string) (string, error) {
	resp, err := c.callAPI("/get_file", map[string]any{"file": file})
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

// getGroupMemberInfo 通过 get_group_member_info 接口查询单个群成员信息
func (c *Client) getGroupMemberInfo(groupID, userID int64) (*GroupMemberInfo, error) {
	resp, err := c.callAPI("/get_group_member_info", map[string]any{
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
func (c *Client) getGroupMemberList(groupID int64) ([]GroupMemberInfo, error) {
	resp, err := c.callAPI("/get_group_member_list", map[string]any{"group_id": groupID})
	if err != nil {
		return nil, err
	}

	var list []GroupMemberInfo
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// fetchFileBytes 获取 napcat 返回的资源字节：
//   - data: base64 内联数据 → 直接解码；
//   - http(s) 地址 → 网络下载；
//   - 其余视为本地文件路径（napcat 对截图等本地缓存会在 URL/File 字段直接返回路径，
//     支持裸路径与 file:// 前缀）。
//
// 不同 NapCat 版本会把同一份资源填进 url / path / file 中的任意一个，
// 因此三种形态都必须能取到字节，否则图片/文件会静默丢失。
func fetchFileBytes(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("资源地址为空")
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return decodeBase64Payload(raw)
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return downloadBytes(raw)
	}
	return os.ReadFile(fileURLPath(raw))
}

// fileURLPath 把 file:// 地址还原为本地路径（兼容 Windows 的 file:///C:/...）
func fileURLPath(raw string) string {
	path := raw
	switch {
	case strings.HasPrefix(path, "file:///"):
		path = path[len("file:///"):]
	case strings.HasPrefix(path, "file://"):
		path = path[len("file://"):]
	}
	return path
}

// decodeBase64Payload 解码 base64 文本（兼容 data URI 前缀与换行）
func decodeBase64Payload(raw string) ([]byte, error) {
	text := strings.TrimSpace(raw)
	if idx := strings.Index(text, "base64,"); idx >= 0 {
		text = text[idx+len("base64,"):]
	}
	text = strings.ReplaceAll(text, "\n", "")
	text = strings.ReplaceAll(text, "\r", "")
	if data, err := base64.StdEncoding.DecodeString(text); err == nil {
		return data, nil
	}
	return base64.RawStdEncoding.DecodeString(text)
}

// downloadBytes 下载 http(s) 资源并返回字节
func downloadBytes(rawURL string) ([]byte, error) {
	return downloadBytesLimited(rawURL, maxMediaDownloadBytes)
}

// downloadBytesLimited 按指定大小上限下载（上限抽参便于测试）
func downloadBytesLimited(rawURL string, maxBytes int64) ([]byte, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("下载地址为空")
	}
	// QQ 媒体是带签名的公开 CDN 直链，原实现也未附带登录态 / 业务 header，
	// 因此这里按地址构建客户端不会改变既有行为，只补上统一的重定向与证书策略。
	resp, err := netsource.Open(rawURL, defaultHTTPTimeout)
	if err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	defer resp.Body.Close()

	buf := &bytes.Buffer{}
	if err := netsource.CopyLimited(buf, resp.Body, maxBytes); err != nil {
		return nil, fmt.Errorf("下载失败: %w", err)
	}
	return buf.Bytes(), nil
}
