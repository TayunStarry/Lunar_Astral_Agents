package engine

import (
	"encoding/json"
	"sync"
	"time"

	"LunarSubsystem/LoggerGeneral"
)

// ==== 动画列表缓存 ====
//
// 由 /write/engine 引擎系统总线（反射观测）更新：引擎侧的 animation_list 消息
// 经 AnimationObserver 观测后写入本缓存，供智能体 getAvailableActions 动态查询。
// 该缓存属于月华的引擎查询能力，因此自持于本包，与传输层解耦。

// ActionDefinition 动作定义（从引擎 ACTION_DEFINITIONS 同步）
type ActionDefinition struct {
	// Name 动作名
	Name string `json:"name"`
	// MouseTracking 是否启用鼠标追踪
	MouseTracking bool `json:"mouseTracking"`
}

// AnimationListCache 动画列表缓存
type AnimationListCache struct {
	sync.RWMutex
	// Actions 可用动作定义
	Actions []ActionDefinition `json:"actions"`
	// UpdatedAt 最近更新时间（毫秒）
	UpdatedAt int64 `json:"updated_at"`
}

// animationListMessage 引擎广播的 animation_list 消息结构（仅解析需要的字段）
type animationListMessage struct {
	Type    string `json:"type"`
	Payload struct {
		ActionDefinitions []ActionDefinition `json:"actionDefinitions"`
	} `json:"payload"`
}

// animCache 动画列表缓存实例
var animCache = &AnimationListCache{}

// GetAnimationCache 获取动画列表缓存（供 getAvailableActions 读取）
func GetAnimationCache() *AnimationListCache {
	return animCache
}

// CacheAnimationList 从引擎消息中提取 animation_list 的动作定义并缓存。
// 非 animation_list 消息被静默忽略。
func CacheAnimationList(msg []byte) {
	var parsed animationListMessage
	if err := json.Unmarshal(msg, &parsed); err != nil {
		return
	}
	if parsed.Type != "animation_list" {
		return
	}
	if len(parsed.Payload.ActionDefinitions) == 0 {
		return
	}

	animCache.Lock()
	defer animCache.Unlock()
	animCache.Actions = parsed.Payload.ActionDefinitions
	animCache.UpdatedAt = time.Now().UnixMilli()
	LoggerGeneral.SubInfo("LunarCore", "EngineBus", "动画列表缓存已更新: %d 个动作", len(animCache.Actions))
}
