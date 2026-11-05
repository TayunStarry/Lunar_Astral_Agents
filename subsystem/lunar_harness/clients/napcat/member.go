package napcat

// member.go 群成员名称缓存：双向 @ 解析与成员列表预热。

import (
	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// CacheMemberName 记录群成员显示名（群名片优先），供双向 @ 解析。
func CacheMemberName(groupID, userID int64, name string) {
	Default().cacheMemberName(groupID, userID, name)
}

// WarmGroupMembers 首次遇到某群消息时异步预取全量成员列表填充名称缓存。
func WarmGroupMembers(groupID int64) {
	Default().warmGroupMembers(groupID)
}

func (c *Client) cacheMemberName(groupID, userID int64, name string) {
	if groupID == 0 || userID == 0 || name == "" {
		return
	}
	c.memberMu.Lock()
	defer c.memberMu.Unlock()
	pool := c.memberNameCache[groupID]
	if pool == nil {
		pool = make(map[int64]string)
		c.memberNameCache[groupID] = pool
	}
	pool[userID] = name
}

// lookupMemberName 从缓存查询群成员显示名
func (c *Client) lookupMemberName(groupID, userID int64) string {
	c.memberMu.Lock()
	defer c.memberMu.Unlock()
	return c.memberNameCache[groupID][userID]
}

// warmGroupMembers 首次遇到某群消息时异步预取全量成员列表填充名称缓存
// （每群每进程一次；发言者与 @ 目标的名称会随消息持续增量补充）
func (c *Client) warmGroupMembers(groupID int64) {
	c.memberMu.Lock()
	already := c.memberListWarmed[groupID]
	c.memberListWarmed[groupID] = true
	c.memberMu.Unlock()
	if already {
		return
	}

	go func() {
		members, err := c.getGroupMemberList(groupID)
		if err != nil {
			// 失败时重置标记，下次消息重新尝试
			c.memberMu.Lock()
			c.memberListWarmed[groupID] = false
			c.memberMu.Unlock()
			return
		}
		for _, member := range members {
			name := member.Card
			if name == "" {
				name = member.Nickname
			}
			c.cacheMemberName(groupID, member.UserID, name)
		}
		logger.SubInfo("LunarCore", "Napcat", "群 %d 成员列表预热完成: %d 人", groupID, len(members))
	}()
}

// resolveGroupMemberName 查询群成员显示名（群名片优先）：缓存 → 接口，查询结果回填缓存
func (c *Client) resolveGroupMemberName(groupID, userID int64) string {
	if name := c.lookupMemberName(groupID, userID); name != "" {
		return name
	}
	info, err := c.getGroupMemberInfo(groupID, userID)
	if err != nil || info == nil {
		return ""
	}
	name := info.Card
	if name == "" {
		name = info.Nickname
	}
	if name != "" {
		c.cacheMemberName(groupID, userID, name)
	}
	return name
}
