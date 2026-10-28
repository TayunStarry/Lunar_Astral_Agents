package DialogueBridging

// type.go 通讯桥接模块的类型定义：上半部为智能体的投递计划与模型调用类型，
// 下半部为 QQ 桥接层（NapCat 协议）的消息与状态类型。

import (
	"encoding/json"
	"time"
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
	Files      []string `json:"files"`       // 文件句柄（@D./@U./@A./@T. 等）
	Reason     string   `json:"reason"`      // 该目标无法执行时的说明
}

// chatMessage OpenAI 兼容的文本消息
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// modelUsage 流式响应中的 token 用量
type modelUsage struct {
	Prompt              int64               `json:"prompt_tokens"`
	Completion          int64               `json:"completion_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
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

// BridgingConfig 桥接器配置，默认从 napcat_config.json 读取
// （旧版 lunar_config.json 的 bridging 组仅作升级兼容）
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

// ==================== 主动投递目标 ====================

// QQGroup QQ 群信息（/get_group_list）
type QQGroup struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
}

// QQFriend QQ 好友信息（/get_friend_list）
type QQFriend struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Remark   string `json:"remark"`
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

// ReplyData 回复消息数据（id 兼容字符串 / 数字两种 JSON 形态）
type ReplyData struct {
	ID json.RawMessage `json:"id"`
}

// ImageData 图片消息数据
//
// QQ 的图片可能走两条通道，字段组合各不相同：
//   - 图片通道（image 段）：file / file_id / url / path 至少有一个可用；
//   - 文件通道（file 段）：以图片扩展名或图片文件头判定，见 processFileSegment。
//
// url 是带时效的 CDN 直链（会过期），path 是 NapCat 本地缓存路径（最可靠），
// file_id 供 get_image 重新拉取。三者都要能兜底，否则某一种组合下图片会静默丢失。
type ImageData struct {
	File     string          `json:"file"`      // 图片文件引用，可传给 get_image
	FileID   string          `json:"file_id"`   // 图片文件 ID（NapCat v4 起下发）
	URL      string          `json:"url"`       // 图片直链（可能过期）
	Path     string          `json:"path"`      // NapCat 本地缓存路径
	FileSize json.RawMessage `json:"file_size"` // 文件大小，可能是数字或字符串
	Summary  string          `json:"summary"`   // 摘要文案（如 "[动画表情]"）
	SubType  int             `json:"sub_type"`  // 子类型（1 表示表情包类图片）
	EmojiID  string          `json:"emoji_id"`  // 商城表情 ID（表情包类图片）
}

// MFaceData 商城表情消息数据（mface 段）
// 部分 NapCat 版本会把它转换成 image 段下发；旧版本仍以 mface 下发，
// 此时没有直链，只能按 summary 渲染为文字占位。
type MFaceData struct {
	EmojiPackageID any    `json:"emoji_package_id"` // 表情包 ID（数字或字符串）
	EmojiID        string `json:"emoji_id"`         // 表情 ID
	Key            string `json:"key"`              // 表情 key
	Summary        string `json:"summary"`          // 表情摘要
	URL            string `json:"url"`              // 部分版本附带的直链
	Path           string `json:"path"`             // 部分版本附带的本地路径
}

// VideoData 视频消息数据
type VideoData struct {
	File     string          `json:"file"`
	URL      string          `json:"url"`
	FileSize json.RawMessage `json:"file_size"` // 可能是数字或字符串
}

// RecordData 语音消息数据（record）
type RecordData struct {
	File     string          `json:"file"`      // 语音文件引用，用于 get_record / get_file
	URL      string          `json:"url"`       // 语音直链
	FileSize json.RawMessage `json:"file_size"` // 可能是数字或字符串
}

// FileData 文件消息数据
//
// NapCat 的文件段按「file 必填 + path/url/name/thumb 可选」下发，
// 不同版本还会附带 file_id / file_name / file_size：
// 这里把所有可能出现的字段都收下，取值时按可用性依次兜底。
type FileData struct {
	File     string          `json:"file"`      // 路径 / URL / file:///
	FileID   string          `json:"file_id"`   // 文件ID，用于 get_file 下载
	FileName string          `json:"file_name"` // 文件名
	Name     string          `json:"name"`      // 文件名（部分版本字段）
	Path     string          `json:"path"`      // 本地路径（收到的文件）
	FileSize json.RawMessage `json:"file_size"` // 文件大小，可能是数字或字符串
	URL      string          `json:"url"`       // 文件直链（可能为空或 base64 data URI）
	Thumb    string          `json:"thumb"`     // 缩略图（图片类文件可用）
}

// ForwardData 合并转发消息数据（id 兼容字符串 / 数字两种 JSON 形态）
type ForwardData struct {
	ID json.RawMessage `json:"id"`
}

// FaceData QQ 表情消息数据（face 段；大表情同样以 face 下发，faceType 标记类型）
type FaceData struct {
	ID  json.RawMessage `json:"id"`  // 表情 id（兼容字符串 / 数字两种形态）
	Raw *FaceRaw        `json:"raw"` // NTQQ 原始表情信息（缺失时为 nil）
}

// FaceRaw NTQQ 表情原始信息，字段用指针以区分"未提供"与"值为零"（faceIndex 0 为合法序号）
type FaceRaw struct {
	FaceIndex *int   `json:"faceIndex"` // NTQQ 表情序号，与 face 段的 id 同源
	FaceText  string `json:"faceText"`  // NTQQ 表情文本，形如 "/呲牙"
	FaceType  int    `json:"faceType"`  // 表情类型（2 为大表情 / 动画表情）
}

// JsonData JSON 卡片消息数据（小程序、邀请、分享卡片等）
type JsonData struct {
	Data string `json:"data"` // 完整 JSON 字符串
}

// ShareData 链接分享消息数据
type ShareData struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	Content string `json:"content"` // 摘要描述
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
	Card     string `json:"card"` // 群名片（部分版本返回）
}

// MessageDetail get_msg 返回的完整消息（用于回复引用还原）
type MessageDetail struct {
	Sender     Sender           `json:"sender"`
	Message    []MessageSegment `json:"message"`
	Content    []MessageSegment `json:"content"` // 部分版本使用 content 字段承载消息段
	RawMessage string           `json:"raw_message"`
}

// GroupMemberInfo 群成员信息（get_group_member_info / get_group_member_list 返回项）
type GroupMemberInfo struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"` // 群名片，可能为空
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
	Path     string `json:"path"`
	FileSize string `json:"file_size"`
	FileName string `json:"file_name"`
	Base64   string `json:"base64"`
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
	Target    BridgeTarget
	Messages  []map[string]any // OpenAI 格式消息列表
	AudioURLs []string         // 语音/音频地址列表，包装为媒体内容项写入智能体统一未读队列
	Trigger   TriggerKind      // 本批次的触发原因
}

// GroupPoolEntry 群聊 / 私聊缓存池中的单条消息
type GroupPoolEntry struct {
	Nickname  string   // 发言用户昵称（群名片优先）
	Content   any      // string 或 []map[string]any（OpenAI 多模态格式）
	HasImages bool     // 是否包含图片
	AudioURLs []string // 语音/音频地址列表
	MessageID int64    // QQ 消息 ID（随机触发自动引用时定位到具体某条消息）
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
