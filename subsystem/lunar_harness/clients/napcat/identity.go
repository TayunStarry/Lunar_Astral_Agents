package napcat

// identity.go 登录账号、群名称与 @ 可读名称的解析与缓存。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
	"encoding/json"
	"strconv"
	"strings"
)

// ResolveBotInfo 返回当前登录账号（get_login_info，成功一次后缓存）。
func ResolveBotInfo() (int64, string) {
	return Default().resolveBotInfo()
}

// ResolveGroupName 解析并缓存群名称。
func ResolveGroupName(groupID int64) string {
	return Default().resolveGroupName(groupID)
}

// ResolveAtDisplay 解析 @ 目标的可读名称：
// all → 全体成员；自己 → 我；群成员 → 群名片/昵称；兜底陌生人昵称，最终退回 QQ 号。
func ResolveAtDisplay(groupID int64, qq string) string {
	return Default().resolveAtDisplay(groupID, qq)
}

// resolveBotInfo 懒加载当前登录账号信息（get_login_info），失败时下次调用重试
func (c *Client) resolveBotInfo() (int64, string) {
	c.botMu.Lock()
	defer c.botMu.Unlock()
	if !c.botInfoLoaded {
		resp, err := c.callAPI("/get_login_info", map[string]any{})
		if err == nil {
			var data struct {
				UserID   int64  `json:"user_id"`
				Nickname string `json:"nickname"`
			}
			if json.Unmarshal(resp.Data, &data) == nil && data.UserID != 0 {
				c.botUserID = data.UserID
				c.botNickname = data.Nickname
				c.botInfoLoaded = true
				logger.SubInfo("LunarCore", "Napcat", "当前账号: %s(%d)", c.botNickname, c.botUserID)
			}
		}
	}
	return c.botUserID, c.botNickname
}

// resolveGroupName 解析并缓存群名称
func (c *Client) resolveGroupName(groupID int64) string {
	c.groupNameMu.Lock()
	if name, ok := c.groupNameCache[groupID]; ok && name != "" {
		c.groupNameMu.Unlock()
		return name
	}
	c.groupNameMu.Unlock()

	name, err := c.getGroupName(groupID)
	if err != nil || name == "" {
		name = strconv.FormatInt(groupID, 10)
	}

	c.groupNameMu.Lock()
	c.groupNameCache[groupID] = name
	c.groupNameMu.Unlock()
	return name
}

// resolveAtDisplay 解析 @ 目标的可读名称
func (c *Client) resolveAtDisplay(groupID int64, qq string) string {
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
	if botID, _ := c.resolveBotInfo(); botID != 0 && userID == botID {
		return "我"
	}
	if groupID != 0 {
		if name := c.resolveGroupMemberName(groupID, userID); name != "" {
			return name
		}
	}
	if name, err := c.getStrangerNickname(userID); err == nil && name != "" {
		return name
	}
	return qq
}
