package napcat

// directory.go 群/好友目录读取与 30 秒缓存。

import (
	"encoding/json"
	"fmt"
	"time"
)

// Directory 读取群与好友目录（30 秒缓存）。
//
// 群/好友接口允许部分成功（例如权限只开了群列表），两个都失败才返回错误。
// 返回的切片是缓存副本，调用方可安全读取。
func Directory() ([]QQGroup, []QQFriend, error) {
	return Default().directory()
}

func (c *Client) directory() ([]QQGroup, []QQFriend, error) {
	c.directoryMu.Lock()
	defer c.directoryMu.Unlock()
	if !c.directoryAt.IsZero() && time.Since(c.directoryAt) < qqDirectoryTTL {
		return append([]QQGroup(nil), c.directoryGroups...), append([]QQFriend(nil), c.directoryFriends...), c.directoryErr
	}

	groups, groupErr := c.fetchGroups()
	friends, friendErr := c.fetchFriends()
	switch {
	case groupErr != nil && friendErr != nil:
		c.directoryErr = fmt.Errorf("读取 QQ 群/好友列表失败（群: %v；好友: %v）", groupErr, friendErr)
	case groupErr != nil:
		c.directoryErr = fmt.Errorf("读取 QQ 群列表失败: %v", groupErr)
	case friendErr != nil:
		c.directoryErr = fmt.Errorf("读取 QQ 好友列表失败: %v", friendErr)
	default:
		c.directoryErr = nil
	}
	c.directoryGroups = groups
	c.directoryFriends = friends
	c.directoryAt = time.Now()
	return append([]QQGroup(nil), groups...), append([]QQFriend(nil), friends...), c.directoryErr
}

// fetchGroups 读取群列表
func (c *Client) fetchGroups() ([]QQGroup, error) {
	resp, err := c.callAPI("/get_group_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var groups []QQGroup
	if err := json.Unmarshal(resp.Data, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// fetchFriends 读取好友列表
func (c *Client) fetchFriends() ([]QQFriend, error) {
	resp, err := c.callAPI("/get_friend_list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var friends []QQFriend
	if err := json.Unmarshal(resp.Data, &friends); err != nil {
		return nil, err
	}
	return friends, nil
}
