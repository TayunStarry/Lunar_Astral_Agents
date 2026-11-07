package DialogueBridging

// type.go 通讯桥接模块的类型定义：上半部为智能体的投递计划与模型调用类型，
// 下半部为 QQ 桥接层（NapCat 协议）的消息与状态类型。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/mediaobject"
)

// ==================== 智能体投递计划类型 ====================

// sendPlan 模型解析出的投递计划（支持一次指定多个目标，按顺序逐个投递）
type sendPlan struct {
	Deliveries []sendDelivery `json:"deliveries"` // 多目标投递清单（新格式）
	Reason     string         `json:"reason"`     // 无法执行时的整体说明

	// 兼容旧单目标格式（模型偶尔仍按旧提示词输出）
	TargetType string   `json:"target_type,omitempty"`
	Target     string   `json:"target,omitempty"`
	Text       string   `json:"text,omitempty"`
	Images     []string `json:"images,omitempty"`
	Files      []string `json:"files,omitempty"`
}

// sendDelivery 单个目标的投递内容
type sendDelivery struct {
	TargetType string   `json:"target_type"` // group / user
	Target     string   `json:"target"`      // 群号 / QQ 号 / 名称
	Text       string   `json:"text"`        // 直接投递的文本（可空）
	Images     []string `json:"images"`      // 图片句柄（@I.xxx）
	Files      []string `json:"files"`       // 文件句柄（@D./@U./@A./@V./@G. 等）
	Reason     string   `json:"reason"`      // 该目标无法执行时的说明
}

// chatMessage OpenAI 兼容的文本消息
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// resolvedTarget 解析后的投递目标
type resolvedTarget struct {
	Kind string // group / user
	ID   int64
	Name string
}

// deliveryOutcome 本轮投递的累计结果
type deliveryOutcome struct {
	TextSent   bool
	ImagesSent int
	FilesSent  int
	Failures   []string
}

// deliveryRecord 最近一次通讯桥接需求（供数据导出）
type deliveryRecord struct {
	Instruction string         `json:"instruction"`
	Deliveries  []sendDelivery `json:"deliveries"`
	Report      string         `json:"report"`
	Success     bool           `json:"success"`
	At          time.Time      `json:"at"`
}

// ==================== 桥接器配置类型 ====================

// BridgingConfig 桥接器配置，从 config/napcat.json 读取
type BridgingConfig struct {
	BridgingType                    string   `json:"bridging_type"`
	BridgingPath                    string   `json:"bridging_path"`
	BridgingToken                   string   `json:"bridging_token"`
	BridgingUsers                   []int64  `json:"bridging_users"`                     // 允许响应的用户QQ号 / 群号列表
	BridgingGroupTriggerProbability *float64 `json:"bridging_group_trigger_probability"` // 群聊未触发关键词时的随机应答概率(0~1)，未配置默认 0.3
	BridgingGroupKeywords           []string `json:"bridging_group_keywords"`            // 群聊触发关键词列表
	// BridgingPrivateTriggerProbability 私聊触发概率(0~1)，未配置默认 1（每条都答）
	// 私聊不再「一发即回」，而是先把连续消息攒成一批；概率只决定是否触发，触发后会等待合并窗口再推送
	BridgingPrivateTriggerProbability *float64 `json:"bridging_private_trigger_probability"`
	// BridgingPrivateBatchWindowMS 私聊连续消息合并窗口(毫秒)，未配置默认 1500
	BridgingPrivateBatchWindowMS *int `json:"bridging_private_batch_window_ms"`
}

// TriggerKind 消息批次被推送给月华的触发原因
type TriggerKind string

const (
	TriggerAt        TriggerKind = "at"        // 被 @
	TriggerKeyword   TriggerKind = "keyword"   // 命中触发关键词
	TriggerRandom    TriggerKind = "random"    // 随机概率触发（群聊需要自动引用最相关消息）
	TriggerRedPacket TriggerKind = "redpacket" // 红包
	TriggerPrivate   TriggerKind = "private"   // 私聊触发（一对一，无需 @ 与引用）
)

// QuoteCandidate 随机触发时可供自动引用的入站消息
type QuoteCandidate struct {
	MessageID int64  // QQ 消息 ID（reply 段的 id）
	Content   string // 消息文本（用于与回应计算相关性）
}

// ==================== 红包感知类型 ====================

// RedPacketInfo 红包感知信息（NapCat 无领取红包接口，仅识别并告知 AI；口令红包可通过复读口令领取）
type RedPacketInfo struct {
	IsRedPacket bool   // 是否为红包消息
	IsPhrase    bool   // 是否为口令红包（可在聊天中复读口令领取）
	Blessing    string // 祝福语 / 口令
	BillNo      string // 红包流水号
}

// ==================== 会话目标与请求类型 ====================

// BridgeTarget 会话目标（月华回应的接收方）
type BridgeTarget struct {
	ID            int64  // 用户QQ号或群号
	IsGroup       bool   // 是否群聊
	GroupName     string // 群名称（群聊时用于消息前缀）
	ReplyToUserID int64  // 群聊回应需要 @ 的发起者（被 @ 触发时设置，首次发言消费一次）
	// QuoteCandidates 随机触发时本批次的可引用消息（首次发言时按相关性挑一条引用，用后清空）
	QuoteCandidates []QuoteCandidate
}

// BridgeRequest 一条待推送给月华的请求（群聊可能包含多条历史消息）
type BridgeRequest struct {
	Target       BridgeTarget
	Messages     []map[string]any     // OpenAI 格式消息列表
	AudioObjects []mediaobject.Object // 音频多模态对象，写入智能体统一未读队列
	Trigger      TriggerKind          // 本批次的触发原因
}

// GroupPoolEntry 群聊 / 私聊缓存池中的单条消息
type GroupPoolEntry struct {
	Nickname     string               // 发言用户昵称（群名片优先）
	Content      any                  // string 或 []map[string]any（OpenAI 多模态格式）
	HasMedia     bool                 // 是否包含多模态内容项（图片 / 动图 / 视频）
	AudioObjects []mediaobject.Object // 音频多模态对象
	MessageID    int64                // QQ 消息 ID（随机触发自动引用时定位到具体某条消息）
}

// GroupPool 单个群聊的消息缓存池（FIFO，保留最新 maxGroupPoolSize 条）
type GroupPool struct {
	GroupID   int64
	GroupName string
	Entries   []GroupPoolEntry
}

// ==================== 桥接器状态类型 ====================

// BridgeState 桥接器连接状态
type BridgeState int

const (
	BridgeDisconnected BridgeState = iota // 未连接
	BridgeConnecting                      // 连接中
	BridgeConnected                       // 已连接
	BridgeFailed                          // 连接失败（达到最大重试次数）
)
