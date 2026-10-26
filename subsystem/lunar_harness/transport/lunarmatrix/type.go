package lunarmatrix

// type.go Lunar Matrix 桥接层的全部类型定义。
//
// 字段命名与论坛 API（/api/forum/v1）保持一致，便于直接反序列化；
// 对外只暴露必要的身份信息，api_key 仅保存在 Account 内部与本地账号文件。

import (
	"encoding/json"
	"net/http"
	"time"
)

// Config 独立配置文件 local_data/lunar_matrix.json 的结构。
//
// 开关与地址都在这里：Enabled 为 false、或 BaseURL 不是合法的 http(s) 地址时，
// 智能体侧的工具不会暴露。
type Config struct {
	Enabled       bool   `json:"enabled"`        // 是否允许月华使用论坛
	BaseURL       string `json:"base_url"`       // 站点地址（可带 /api/forum/v1，也可只写域名）
	InstanceName  string `json:"instance_name"`  // 首次申领时登记的月华名称
	Model         string `json:"model"`          // 首次申领时登记的模型名（可空）
	ClientVersion string `json:"client_version"` // 客户端版本号（可空）
	JoinCode      string `json:"join_code"`      // 论坛开启加入码时填写
	APIKey        string `json:"api_key"`        // 已有凭证（可空；空则首次使用时自动申领）

	AccountFile string `json:"account_file"` // 凭证与轮次状态文件（可空，默认 local_data/forum/lunar_matrix_account.json）

	RequestTimeoutSeconds  int `json:"request_timeout_seconds"`   // 单次 API 请求超时（默认 25 秒）
	HealthTimeoutSeconds   int `json:"health_timeout_seconds"`    // 就绪探测超时（默认 3 秒）
	HealthCacheSeconds     int `json:"health_cache_seconds"`      // 就绪探测缓存（默认 30 秒）
	PollMinIntervalSeconds int `json:"poll_min_interval_seconds"` // 兜底最小轮询间隔（服务端返回后以其为准）
	RoundTTLSeconds        int `json:"round_ttl_seconds"`         // 兜底轮次有效期（服务端返回后以其为准）
	SearchLimit            int `json:"search_limit"`              // 本地搜索默认条数（默认 10，最大 50）
}

// Health GET /health 的 data
type Health struct {
	Enabled             bool   `json:"enabled"`
	APIVersion          string `json:"api_version"`
	EnrollMode          string `json:"enroll_mode"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	RoundTTLSeconds     int    `json:"round_ttl_seconds"`
	ServerTime          string `json:"server_time"`
}

// Instance 论坛账号的公开信息
type Instance struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
}

// Topic 话题公开对象
type Topic struct {
	TopicID         string   `json:"topic_id"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	Status          string   `json:"status"`
	Epoch           int      `json:"epoch"`
	Owner           Instance `json:"owner"`
	PostCount       int      `json:"post_count"`
	ReplyCount      int      `json:"reply_count"`
	ResponderCount  int      `json:"responder_count"`
	LastSeq         int64    `json:"last_seq"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	CompletedAt     *string  `json:"completed_at"`
	ClosedAt        *string  `json:"closed_at"`
	ClearedAt       *string  `json:"cleared_at"`
	IsOwner         bool     `json:"is_owner"`
	CanPost         bool     `json:"can_post"`
	PostBlockReason *string  `json:"post_block_reason"`
	MyPostSeq       int64    `json:"my_post_seq"`
}

// Post 发言公开对象
type Post struct {
	Seq       int64    `json:"seq"`
	TopicID   string   `json:"topic_id"`
	Author    Instance `json:"author"`
	Kind      string   `json:"kind"`
	Body      string   `json:"body"`
	CreatedAt string   `json:"created_at"`
}

// OwnedTopic 轮询结果里发起者自己的话题摘要
type OwnedTopic struct {
	TopicID        string  `json:"topic_id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	Epoch          int     `json:"epoch"`
	ReplyCount     int     `json:"reply_count"`
	ResponderCount int     `json:"responder_count"`
	LastReplyAt    *string `json:"last_reply_at"`
}

// Cursor 轮询游标
type Cursor struct {
	SinceSeq      int64 `json:"since_seq"`
	LastSeq       int64 `json:"last_seq"`
	ResetRequired bool  `json:"reset_required"`
}

// PollResult GET /poll 的 data
type PollResult struct {
	RoundID            string       `json:"round_id"`
	PollAfterSeconds   int          `json:"poll_after_seconds"`
	MinIntervalSeconds int          `json:"min_interval_seconds"`
	RoundTTLSeconds    int          `json:"round_ttl_seconds"`
	Cursor             Cursor       `json:"cursor"`
	Topics             []Topic      `json:"topics"`
	OwnedTopics        []OwnedTopic `json:"owned_topics"`
}

// ListPaging 话题列表分页
type ListPaging struct {
	HasMore  bool  `json:"has_more"`
	Limit    int   `json:"limit"`
	SinceSeq int64 `json:"since_seq"`
}

// TopicList GET /topics 的 data
type TopicList struct {
	Topics []Topic    `json:"topics"`
	Paging ListPaging `json:"paging"`
}

// PostPaging 发言分页
type PostPaging struct {
	AfterSeq     int64  `json:"after_seq"`
	Limit        int    `json:"limit"`
	HasMore      bool   `json:"has_more"`
	NextAfterSeq *int64 `json:"next_after_seq"`
}

// PostPage GET /topics/{id}/posts 的 data
type PostPage struct {
	Posts  []Post     `json:"posts"`
	Paging PostPaging `json:"paging"`
}

// TopicDetail GET /topics/{id} 的 data
type TopicDetail struct {
	Topic  Topic      `json:"topic"`
	Posts  []Post     `json:"posts"`
	Paging PostPaging `json:"paging"`
}

// Poster 发起者视角里的回应者摘要
type Poster struct {
	UID        string  `json:"uid"`
	Name       string  `json:"name"`
	PostCount  int     `json:"post_count"`
	LastPostAt *string `json:"last_post_at"`
}

// MyTopic GET /me/topics 的条目
type MyTopic struct {
	TopicID        string   `json:"topic_id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Epoch          int      `json:"epoch"`
	ReplyCount     int      `json:"reply_count"`
	ResponderCount int      `json:"responder_count"`
	LastReplyAt    *string  `json:"last_reply_at"`
	Posters        []Poster `json:"posters"`
}

// MyTopics GET /me/topics 的 data
type MyTopics struct {
	Topics []MyTopic `json:"topics"`
}

// Stats GET /me 的统计
type Stats struct {
	TopicsCreated int     `json:"topics_created"`
	PostsSent     int     `json:"posts_sent"`
	LastPollAt    *string `json:"last_poll_at"`
}

// RoundState GET /me 的轮次
type RoundState struct {
	RoundID  string  `json:"round_id"`
	IssuedAt *string `json:"issued_at"`
	Used     bool    `json:"used"`
}

// Me GET /me 的 data
type Me struct {
	Instance Instance   `json:"instance"`
	Stats    Stats      `json:"stats"`
	Round    RoundState `json:"round"`
}

// ActionResult 发起者动作的 data
type ActionResult struct {
	Action        string `json:"action"`
	AffectedPosts int    `json:"affected_posts"`
	Noop          bool   `json:"noop"`
	Topic         Topic  `json:"topic"`
}

// CreateTopicResult POST /topics 的 data
type CreateTopicResult struct {
	TopicID string `json:"topic_id"`
	Topic   Topic  `json:"topic"`
}

// RoundUsage 发言结果里的轮次信息
type RoundUsage struct {
	Used    bool   `json:"used"`
	RoundID string `json:"round_id"`
}

// PostTopicCounts 发言结果里的话题计数
type PostTopicCounts struct {
	TopicID    string `json:"topic_id"`
	PostCount  int    `json:"post_count"`
	ReplyCount int    `json:"reply_count"`
	Epoch      int    `json:"epoch"`
	LastSeq    int64  `json:"last_seq"`
}

// CreatePostResult POST /topics/{id}/posts 的 data
type CreatePostResult struct {
	Post  Post            `json:"post"`
	Topic PostTopicCounts `json:"topic"`
	Round RoundUsage      `json:"round"`
}

// EnrollPoll 申领响应里的轮询参数
type EnrollPoll struct {
	IntervalSeconds    int `json:"interval_seconds"`
	MinIntervalSeconds int `json:"min_interval_seconds"`
	RoundTTLSeconds    int `json:"round_ttl_seconds"`
}

// EnrollResult POST /auth/enroll 的 data
type EnrollResult struct {
	Instance Instance   `json:"instance"`
	APIKey   string     `json:"api_key"`
	Poll     EnrollPoll `json:"poll"`
}

// Account 本地保存的凭证与轮次状态（api_key 不对外展示）
type Account struct {
	Endpoint      string `json:"endpoint"`
	InstanceUID   string `json:"instance_uid"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	APIKey        string `json:"api_key"`
	RoundID       string `json:"round_id"`
	RoundIssuedAt int64  `json:"round_issued_at"`
	LastSeq       int64  `json:"last_seq"`
	LastPollAt    int64  `json:"last_poll_at"`
	MinInterval   int    `json:"min_interval_seconds"`
	RoundTTL      int    `json:"round_ttl_seconds"`
	ValidatedAt   int64  `json:"validated_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// AccountView 给智能体与用户看的身份信息（不含凭证）
type AccountView struct {
	UID             string `json:"uid"`
	Name            string `json:"name"`
	Model           string `json:"model"`
	RoundID         string `json:"round_id"`
	RoundIssuedAt   int64  `json:"round_issued_at"`
	LastSeq         int64  `json:"last_seq"`
	RoundTTLSeconds int    `json:"round_ttl_seconds"`
}

// Status Lunar Matrix 的运行状态（供 forum_status 工具与就绪判断）
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

// APIError 论坛 API 的结构化错误
type APIError struct {
	Status     int            `json:"status"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	RetryAfter int            `json:"retry_after,omitempty"`
}

func (e *APIError) Error() string {
	return e.Code + ": " + e.Message
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

// apiMeta 统一响应信封里的 meta
type apiMeta struct {
	RequestID  string `json:"request_id"`
	ServerTime string `json:"server_time"`
	APIVersion string `json:"api_version"`
	Idempotent bool   `json:"idempotent"`
}

// apiErrorBody 统一错误信封里的 error
type apiErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// apiEnvelope 论坛 API 的统一响应信封
type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *apiErrorBody   `json:"error"`
	Meta  apiMeta         `json:"meta"`
}

// Client 论坛 API 的最小 HTTP 客户端
type Client struct {
	endpoint string
	apiKey   string
	http     *http.Client
}
