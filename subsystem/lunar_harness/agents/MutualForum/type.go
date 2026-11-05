package MutualForum

// type.go Mutual Forum（互助论坛）智能体与桥接层的类型定义。
//
// API DTO 与账号持久化结构已下沉到 engine/forum（纯 HTTP、序列化与文件存储层），
// 本包只保留：
//   - 模型调用相关的消息/工具类型；
//   - 本地配置；
//   - 面向模型与用户的状态视图。
//
// 桥接层字段命名与论坛 API（/api/forum/v1）保持一致，便于直接反序列化；
// 对外只暴露必要的身份信息，api_key 仅保存在 Account 内部与本地账号文件。

import (
	"time"

	"LunarSubsystem/LunarHarness/engine/forum"
	"LunarSubsystem/LunarHarness/protocol/agentbase"
)

// ==== API DTO（复用 engine/forum，避免两套字段漂移） ====

type (
	Health              = forum.Health
	Instance            = forum.Instance
	Authorship          = forum.Authorship
	Moderation          = forum.Moderation
	Guidance            = forum.Guidance
	Topic               = forum.Topic
	Post                = forum.Post
	OwnedTopic          = forum.OwnedTopic
	Cursor              = forum.Cursor
	UpdateCursor        = forum.UpdateCursor
	PollResult          = forum.PollResult
	ListPaging          = forum.ListPaging
	TopicList           = forum.TopicList
	PostPaging          = forum.PostPaging
	PostPage            = forum.PostPage
	TopicDetail         = forum.TopicDetail
	Poster              = forum.Poster
	MyTopic             = forum.MyTopic
	MyPaging            = forum.MyPaging
	MyTopics            = forum.MyTopics
	Stats               = forum.Stats
	Limits              = forum.Limits
	RoundState          = forum.RoundState
	Me                  = forum.Me
	ActionResult        = forum.ActionResult
	CreateTopicResult   = forum.CreateTopicResult
	RoundUsage          = forum.RoundUsage
	PostTopicCounts     = forum.PostTopicCounts
	CreatePostResult    = forum.CreatePostResult
	EnrollPoll          = forum.EnrollPoll
	EnrollResult        = forum.EnrollResult
	Comment             = forum.Comment
	CommentPaging       = forum.CommentPaging
	CommentPage         = forum.CommentPage
	CreateCommentResult = forum.CreateCommentResult
	ReactResult         = forum.ReactResult
	DeleteResult        = forum.DeleteResult
	PollOption          = forum.PollOption
	PollView            = forum.PollView
	PollResults         = forum.PollResults
	UpdateEvent         = forum.UpdateEvent
	UpdateFeed          = forum.UpdateFeed
	GuidanceItem        = forum.GuidanceItem
	GuidanceList        = forum.GuidanceList
	APIError            = forum.APIError
	Client              = forum.Client
)

// ==== 智能体模型调用类型 ====

// chatMessage 独立上下文中的消息（仅角色与文本，供历史快照与测试解码）
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []toolCallItem `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// toolCallItem 模型返回的工具调用（统一复用基座类型）
type toolCallItem = agentbase.ToolCall

// toolCallFunction 工具名与 JSON 参数（统一复用基座类型）
type toolCallFunction = agentbase.ToolFunction

// toolDefinition 发给模型的函数工具定义（统一复用基座类型）
type toolDefinition = agentbase.ToolDefinition

// toolFunction 函数描述与参数 Schema（统一复用基座类型）
type toolFunction = agentbase.ToolFunctionDef

// toolOutcome 工具执行结果：JSON 回填 + 给最终总结用的短句
type toolOutcome struct {
	JSON    string
	Summary string
}

// ==== 本地配置与账号状态视图 ====

// Config 独立配置文件 local_data/config/forum.json 的结构。
//
// 开关与地址都在这里：Enabled 为 false、或 BaseURL 不是合法的 http(s) 地址时，
// 智能体侧的工具不会暴露。
//
// v2 迁移口径：model / client_version 已从申领入参移除，本地旧键会被 JSON 解码忽略，
// 不再读取也不发送；join_code 语义改为「测试豁免码」，不填或填错仍可正常申领。
type Config struct {
	Enabled      bool   `json:"enabled"`       // 是否允许月华使用论坛
	BaseURL      string `json:"base_url"`      // 站点地址（可带 /api/forum/v1，也可只写域名）
	InstanceName string `json:"instance_name"` // 首次申领时登记的月华名称
	AutoName     bool   `json:"auto_name"`     // 首次申领时由月华自己生成网名（默认 true）
	JoinCode     string `json:"join_code"`     // 可选测试豁免码；正确时跳过发帖限额，不填不阻塞申领
	QQ           string `json:"qq"`            // 可选 QQ 号：仅首次申领用于身份绑定与头像派生，不落盘到账号文件
	UID          string `json:"uid"`           // 可选稳定标识（上次申领返回的 instance.uid）
	APIKey       string `json:"api_key"`       // 已有凭证（可空；空则首次使用时自动申领）

	AccountFile string `json:"account_file"` // 凭证与轮次状态文件（可空，默认 local_data/database/forum/forum_account.json）

	RequestTimeoutSeconds  int `json:"request_timeout_seconds"`   // 单次 API 请求超时（默认 25 秒）
	HealthTimeoutSeconds   int `json:"health_timeout_seconds"`    // 就绪探测超时（默认 3 秒）
	HealthCacheSeconds     int `json:"health_cache_seconds"`      // 就绪探测缓存（默认 30 秒）
	PollMinIntervalSeconds int `json:"poll_min_interval_seconds"` // 兜底最小轮询间隔（服务端返回后以其为准）
	RoundTTLSeconds        int `json:"round_ttl_seconds"`         // 兜底轮次有效期（服务端返回后以其为准）
	SearchLimit            int `json:"search_limit"`              // 本地搜索默认条数（默认 10，最大 50）
}

// Account 本地保存的凭证与轮次状态（api_key 不对外展示）。
//
// 结构与磁盘格式由 engine/forum 定义并负责读写、迁移与 .bak 回滚；本包只消费字段。
type Account = forum.Account

// AccountView 给智能体与用户看的身份信息（不含凭证）。
type AccountView struct {
	UID             string `json:"uid"`
	Name            string `json:"name"`
	NameGenerated   bool   `json:"name_generated"`
	QQMAsked        string `json:"qq_masked,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	AvatarFallback  string `json:"avatar_fallback,omitempty"`
	Kind            string `json:"kind,omitempty"`
	TestMode        bool   `json:"test_mode"`
	RoundID         string `json:"round_id"`
	RoundUsed       bool   `json:"round_used"`
	RoundIssuedAt   int64  `json:"round_issued_at"`
	LastSeq         int64  `json:"last_seq"`
	EventSeq        int64  `json:"event_seq"`
	RoundTTLSeconds int    `json:"round_ttl_seconds"`
}

// Status Mutual Forum 的运行状态（供 forum_status 工具与就绪判断）
type Status struct {
	Enabled       bool         `json:"enabled"`
	Reachable     bool         `json:"reachable"`
	BaseURL       string       `json:"base_url"`
	Error         string       `json:"error,omitempty"`
	IdentityError string       `json:"identity_error,omitempty"`
	Health        *Health      `json:"health,omitempty"`
	Identity      *AccountView `json:"identity,omitempty"`
	Me            *Me          `json:"me,omitempty"`
}

// SearchHit 本地检索命中
type SearchHit struct {
	Topic     Topic    `json:"topic"`
	Score     int      `json:"score"`
	MatchedIn []string `json:"matched_in"`
	Posts     []Post   `json:"posts,omitempty"`
}

// readinessProbe 就绪探测缓存（package 内部使用）
type readinessProbe struct {
	Endpoint  string
	OK        bool
	Health    Health
	Error     string
	CheckedAt time.Time
}
