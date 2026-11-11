package bilibili

// follow.go 关注分组接口：读取「特别关注」成员。
//
// B 站关注分组接口 x/relation/tag 里「特别关注」是固定 tagid=-10（默认分组为 0）。

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SpecialFollows 读取当前账号「特别关注」分组里的用户。
func SpecialFollows(f Fetcher) ([]MentionTarget, error) {
	target := fmt.Sprintf("%s?tagid=%d&pn=1&ps=%d", followTagURL, specialFollowTagID, specialFollowPageSize)
	raw, err := f.FetchText("GET", target, "", "", followTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    []struct {
			Mid   int64  `json:"mid"`
			Uname string `json:"uname"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("解析特别关注列表失败: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("特别关注接口 code=%d %s", resp.Code, resp.Message)
	}
	out := make([]MentionTarget, 0, len(resp.Data))
	for _, user := range resp.Data {
		name := strings.TrimSpace(user.Uname)
		if name == "" || user.Mid <= 0 {
			continue
		}
		out = append(out, MentionTarget{Name: name, UID: user.Mid})
	}
	return out, nil
}
