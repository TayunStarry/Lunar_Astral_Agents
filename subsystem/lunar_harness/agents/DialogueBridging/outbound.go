package DialogueBridging

// outbound.go 主动投递的桥接层入口：目标目录读取与就绪判定。
//
// 群/好友目录与文件/文本发送由 clients/napcat 负责；这里只保留「桥接器是否
// 已连接、能否投递」的业务语义。

import (
	"fmt"

	"LunarSubsystem/LunarHarness/clients/napcat"
)

// ==================== 目标目录 ====================

// QQTargets 读取群与好友目录（30 秒缓存，由 clients/napcat 维护）。
//
// 桥接器未连接时立即失败；群/好友接口允许部分成功（例如权限只开了群列表），
// 两个都失败才返回错误。返回的切片是缓存副本，调用方可安全读取。
func QQTargets() ([]napcat.QQGroup, []napcat.QQFriend, error) {
	if GetBridgeState() != BridgeConnected {
		return nil, nil, fmt.Errorf("QQ 桥接器未连接")
	}
	return napcat.Directory()
}

// BridgeReady 主动投递是否可用：连接成功 + 至少能解析出一个群/好友。
// 群列表或好友列表单独失败时，只要另一侧能解析出目标就算可用。
// 目录接口本身要求已登录，因此不再额外调用 get_login_info（避免每轮多一次接口）。
func BridgeReady() bool {
	if GetBridgeState() != BridgeConnected {
		return false
	}
	groups, friends, _ := QQTargets()
	return len(groups)+len(friends) > 0
}
