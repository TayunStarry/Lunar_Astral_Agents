package bilibili

// dynamic.go 发布纯文本动态（type=4）。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PublishDynamic 把一段纯文本作为动态发布，返回 dynamic_id。
func PublishDynamic(p Page, text string) (string, error) {
	csrf := p.CSRFToken()
	if csrf == "" {
		return "", fmt.Errorf("页面里读不到 bili_jct，无法发动态")
	}
	clean := strings.TrimSpace(text)
	if clean == "" {
		return "", fmt.Errorf("动态正文为空")
	}
	fields := map[string]string{
		"dynamic_id":        "0",
		"type":              "4",
		"rid":               "0",
		"content":           clean,
		"up_choose_comment": "0",
		"up_close_comment":  "0",
		"extension":         `{"emoji_type":1,"from":{"emoji_type":1},"flag_cfg":{}}`,
		"csrf":              csrf,
		"csrf_token":        csrf,
	}
	raw, err := p.FetchForm(dynamicCreateURL, fields, dynamicTimeout)
	if err != nil {
		return "", err
	}
	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			DynamicID    int64  `json:"dynamic_id"`
			DynamicIDStr string `json:"dynamic_id_str"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return "", err
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("B站拒绝动态: code=%d %s", resp.Code, resp.Msg)
	}
	id := resp.Data.DynamicIDStr
	if id == "" && resp.Data.DynamicID != 0 {
		id = fmt.Sprintf("%d", resp.Data.DynamicID)
	}
	if id == "" {
		return "", fmt.Errorf("动态已提交但没有返回 dynamic_id")
	}
	return id, nil
}
