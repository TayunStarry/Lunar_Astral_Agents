package creation

// plan_qq.go QQ 适配器可用性的只读探测。

import (
	"strconv"
	"strings"

	"LunarSubsystem/LunarHarness/clients/napcat"
)

// probeQQTargets 只读探测 NapCat 适配器：能读到群 / 好友目录即视为已连接。
// 适配器未连接时接口调用会失败，这里折叠为空集合，由选题逻辑跳过 QQ 事项，
// 不向调用方抛错，避免一个不可用能力中断整轮创作事项选择。
func probeQQTargets() qqTargets {
	groups, friends, err := napcat.Directory()
	if err != nil && len(groups) == 0 && len(friends) == 0 {
		return qqTargets{}
	}
	items := make([]qqTarget, 0, len(groups)+len(friends))
	for _, group := range groups {
		if group.GroupID <= 0 {
			continue
		}
		items = append(items, qqTarget{Group: true, ID: group.GroupID, Name: qqName(group.GroupName, group.GroupID)})
	}
	for _, friend := range friends {
		if friend.UserID <= 0 {
			continue
		}
		name := friend.Remark
		if strings.TrimSpace(name) == "" {
			name = friend.Nickname
		}
		items = append(items, qqTarget{ID: friend.UserID, Name: qqName(name, friend.UserID)})
	}
	return qqTargets{Items: items}
}

// qqName 名称缺失时退回编号，保证事项文案里的目标可识别。
func qqName(name string, id int64) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	return "QQ-" + strconv.FormatInt(id, 10)
}
