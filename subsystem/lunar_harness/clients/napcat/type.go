package napcat

// type.go NapCat / OneBot 协议类型与传输客户端状态。

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Config 传输客户端配置：WebSocket 地址与鉴权 token。
type Config struct {
	Endpoint string
	Token    string
}

// Client NapCat 传输客户端：持有端点、HTTP 客户端与各接口缓存。
type Client struct {
	endpoint string
	token    string

	httpClient       *http.Client
	uploadHTTPClient *http.Client

	groupNameMu    sync.Mutex
	groupNameCache map[int64]string

	memberMu         sync.Mutex
	memberNameCache  map[int64]map[int64]string
	memberListWarmed map[int64]bool

	botMu         sync.Mutex
	botInfoLoaded bool
	botUserID     int64
	botNickname   string

	directoryMu      sync.Mutex
	directoryAt      time.Time
	directoryGroups  []QQGroup
	directoryFriends []QQFriend
	directoryErr     error
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

// WalletElement NTQQ 钱包元素（红包），承载于 raw.elements[].walletElement
type WalletElement struct {
	RedType       int            `json:"redType"`       // 1=群聊手气/口令, 2=私聊/普通
	BillNo        string         `json:"billNo"`        // 红包流水号
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
