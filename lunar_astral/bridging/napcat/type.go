package napcat

import (
	"encoding/json"
)

// ==================== 桥接器配置类型 ====================

// BridgingConfig 桥接器配置，从 lunar_config.json 的 server 段读取
type BridgingConfig struct {
	BridgingType                    string   `json:"bridging_type"`
	BridgingPath                    string   `json:"bridging_path"`
	BridgingToken                   string   `json:"bridging_token"`
	BridgingUsers                   []int64  `json:"bridging_users"`                     // 允许响应的用户QQ号 / 群号列表
	BridgingGroupTriggerProbability *float64 `json:"bridging_group_trigger_probability"` // 群聊未触发关键词时的随机应答概率(0~1)，未配置默认 0.3
	BridgingGroupKeywords           []string `json:"bridging_group_keywords"`            // 群聊触发关键词列表
}

// ==================== Napcat 消息类型 ====================

// NapcatMessage Napcat 上行消息结构（私聊或群聊）
type NapcatMessage struct {
	SelfID      int64            `json:"self_id"`
	UserID      int64            `json:"user_id"`
	GroupID     int64            `json:"group_id"` // 群聊时为群号，私聊时为 0
	MessageID   int64            `json:"message_id"`
	Sender      Sender           `json:"sender"`
	PostType    string           `json:"post_type"`
	MessageType string           `json:"message_type"`
	Message     []MessageSegment `json:"message"`
	RawMessage  string           `json:"raw_message"`
	Raw         json.RawMessage  `json:"raw"` // NTQQ 原始消息，红包等特殊消息承载于此
}

// Sender 发送者信息
type Sender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"` // 群名片或私聊备注名
}

// MessageSegment 消息段
type MessageSegment struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// TextData 文本消息数据
type TextData struct {
	Text string `json:"text"`
}

// AtData @消息数据
type AtData struct {
	QQ string `json:"qq"`
}

// ReplyData 回复消息数据
type ReplyData struct {
	ID string `json:"id"`
}

// ImageData 图片消息数据
type ImageData struct {
	File string `json:"file"` // 图片文件引用，用于 get_image 下载
	URL  string `json:"url"`  // 图片直链
}

// VideoData 视频消息数据
type VideoData struct {
	File     string          `json:"file"`
	URL      string          `json:"url"`
	FileSize json.RawMessage `json:"file_size"` // 可能是数字或字符串
}

// FileData 文件消息数据
type FileData struct {
	File     string          `json:"file"`      // 文件名或文件引用
	FileID   string          `json:"file_id"`   // 文件ID，用于 get_file 下载
	FileName string          `json:"file_name"` // 文件名
	Name     string          `json:"name"`      // 文件名（部分版本字段）
	FileSize json.RawMessage `json:"file_size"` // 文件大小，可能是数字或字符串
	URL      string          `json:"url"`       // 文件直链（可能为空）
}

// ForwardData 合并转发消息数据
type ForwardData struct {
	ID string `json:"id"`
}

// RedPacketInfo 红包感知信息（NapCat 无领取红包接口，仅识别并告知 AI；口令红包可通过复读口令领取）
type RedPacketInfo struct {
	IsRedPacket bool   // 是否为红包消息
	IsPhrase    bool   // 是否为口令红包（可在聊天中复读口令领取）
	Blessing    string // 祝福语 / 口令
	BillNo      string // 红包流水号
}

// WalletElement NTQQ 钱包元素（红包），承载于 raw.elements[].walletElement
type WalletElement struct {
	RedType       int            `json:"redType"`       // 1=群聊手气/口令, 2=私聊/普通
	BillNo        string         `json:"billNo"`        // 红包流水号
	Authkey       string         `json:"authkey"`       // 领取密钥（当前未使用）
	Sessiontype   int            `json:"sessiontype"`   // 0=私聊, 1=群聊
	MsgType       int            `json:"msgType"`       // 2=普通/私聊, 3=手气, 6=口令
	RedChannel    int            `json:"redChannel"`    // 1=普通/手气, 32=口令
	GrabbedAmount string         `json:"grabbedAmount"` // 已领取金额
	Receiver      WalletReceiver `json:"receiver"`      // 红包卡片文案
}

// WalletReceiver 红包卡片接收方文案
type WalletReceiver struct {
	Title   string `json:"title"`   // 祝福语 / 口令
	Notice  string `json:"notice"`  // 形如 "[QQ红包]恭喜发财"
	Content string `json:"content"` // 固定为 "QQ红包"
}

// ForwardMessage 合并转发消息中的单条消息
type ForwardMessage struct {
	Sender  ForwardSender    `json:"sender"`
	Message []MessageSegment `json:"message"`
	Content []MessageSegment `json:"content"` // 部分版本使用 content 字段承载消息段
}

// ForwardSender 合并转发消息发送者
type ForwardSender struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
}

// NapcatWSResponse Napcat HTTP API 响应结构
type NapcatWSResponse struct {
	Status  string          `json:"status"`
	Retcode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

// GetFileResponse get_file / get_image 等文件接口的 data 响应
type GetFileResponse struct {
	File     string `json:"file"`
	URL      string `json:"url"`
	FileSize string `json:"file_size"`
	FileName string `json:"file_name"`
	Base64   string `json:"base64"`
}

// ==================== 会话目标与请求类型 ====================

// BridgeTarget 会话目标（月华回应的接收方）
type BridgeTarget struct {
	ID        int64  // 用户QQ号或群号
	IsGroup   bool   // 是否群聊
	GroupName string // 群名称（群聊时用于消息前缀）
}

// BridgeRequest 一条待推送给月华的请求（群聊可能包含多条历史消息）
type BridgeRequest struct {
	Target    BridgeTarget
	Messages  []map[string]interface{} // OpenAI 格式消息列表
	VideoURLs []string                 // 视频地址列表，写入智能体 unreadVideoUrl
}

// GroupPoolEntry 群聊缓存池中的单条消息
type GroupPoolEntry struct {
	Nickname  string      // 发言用户昵称（群名片优先）
	Content   interface{} // string 或 []map[string]interface{}（OpenAI 多模态格式）
	HasImages bool        // 是否包含图片
	VideoURLs []string    // 视频地址列表
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
