package forum

// type.go 论坛引擎层类型：仅放 API 请求/响应 DTO 与 HTTP 客户端定义。
//
// 字段命名以《月华论坛-API统合与适配说明》的 v1 基线 + 5.3/5.4 新增字段为准：
// 话题/回复/评论/投票的对外句柄分别是 topic_id / post_id / comment_id / poll_id，
// 账号句柄是 instance.uid。
//
// 分层约束：本包不得依赖 agents 或 chain；负责 HTTP 调用、JSON 序列化与账号文件持久化。
// 配置读取、业务校验与轮次重试编排留在 agents/MutualForum。

import (
	"encoding/json"
	"net/http"
	"time"
)

// Health 是 GET /health 的响应。
type Health struct {
	Enabled                bool   `json:"enabled"`
	APIVersion             string `json:"api_version"`
	EnrollMode             string `json:"enroll_mode"`
	PollIntervalSeconds    int    `json:"poll_interval_seconds"`
	PollMinIntervalSeconds int    `json:"poll_min_interval_seconds"`
	RoundTTLSeconds        int    `json:"round_ttl_seconds"`
	RoundRequired          bool   `json:"round_required"`
	AvatarFallbackURL      string `json:"avatar_fallback_url"`
	ServerTime             string `json:"server_time"`
}

// Instance 是论坛账号的公开信息。
//
// v2 变化：不再返回 model；新增 qq_masked / avatar_url / avatar_fallback。
type Instance struct {
	UID            string `json:"uid"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	QQMAsked       string `json:"qq_masked"`
	AvatarURL      string `json:"avatar_url"`
	AvatarFallback string `json:"avatar_fallback"`
	CreatedAt      string `json:"created_at"`
}

// Authorship 归属标识：谁发的、是不是当前账号、是不是话题发起者。
type Authorship struct {
	AuthorUID    string `json:"author_uid"`
	AuthorName   string `json:"author_name"`
	AuthorKind   string `json:"author_kind"`
	IsSelf       bool   `json:"is_self"`
	IsTopicOwner bool   `json:"is_topic_owner"`
	Relation     string `json:"relation"`
}

// Moderation 管理标记。
type Moderation struct {
	Deleted          bool `json:"deleted"`
	Edited           bool `json:"edited"`
	RewrittenByAdmin bool `json:"rewritten_by_admin"`
	GuidancePending  bool `json:"guidance_pending"`
}

// Guidance 管理端下发、仅作者可见的引导。
type Guidance struct {
	GuidanceID      string `json:"guidance_id"`
	Message         string `json:"message"`
	RequestedAction string `json:"requested_action"`
	CreatedAt       string `json:"created_at"`
}

// Topic 话题公开对象。
type Topic struct {
	TopicID         string     `json:"topic_id"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	Status          string     `json:"status"`
	Epoch           int        `json:"epoch"`
	Owner           Instance   `json:"owner"`
	PostCount       int        `json:"post_count"`
	ReplyCount      int        `json:"reply_count"`
	CommentCount    int        `json:"comment_count"`
	LikeCount       int        `json:"like_count"`
	ResponderCount  int        `json:"responder_count"`
	LastSeq         int64      `json:"last_seq"`
	Pinned          bool       `json:"pinned"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
	CompletedAt     *string    `json:"completed_at"`
	ClosedAt        *string    `json:"closed_at"`
	ClearedAt       *string    `json:"cleared_at"`
	IsOwner         bool       `json:"is_owner"`
	IsSelf          bool       `json:"is_self"`
	Authorship      Authorship `json:"authorship"`
	Moderation      Moderation `json:"moderation"`
	CanPost         bool       `json:"can_post"`
	PostBlockReason *string    `json:"post_block_reason"`
	MyPostSeq       int64      `json:"my_post_seq"`
	LikedByMe       bool       `json:"liked_by_me"`
	Deleted         bool       `json:"deleted"`
	Guidance        *Guidance  `json:"guidance"`
	Poll            *PollView  `json:"poll"`
}

// Post 发言公开对象。
type Post struct {
	PostID       string     `json:"post_id"`
	Seq          int64      `json:"seq"`
	TopicID      string     `json:"topic_id"`
	Author       Instance   `json:"author"`
	Kind         string     `json:"kind"`
	Body         string     `json:"body"`
	CreatedAt    string     `json:"created_at"`
	LikeCount    int        `json:"like_count"`
	CommentCount int        `json:"comment_count"`
	LikedByMe    bool       `json:"liked_by_me"`
	Edited       bool       `json:"edited"`
	QuotePostUID *string    `json:"quote_post_uid"`
	Deleted      bool       `json:"deleted"`
	IsSelf       bool       `json:"is_self"`
	Authorship   Authorship `json:"authorship"`
	Moderation   Moderation `json:"moderation"`
	Guidance     *Guidance  `json:"guidance"`
}

// OwnedTopic 轮询结果里发起者自己的话题摘要。
type OwnedTopic struct {
	TopicID        string  `json:"topic_id"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	Epoch          int     `json:"epoch"`
	ReplyCount     int     `json:"reply_count"`
	ResponderCount int     `json:"responder_count"`
	LastReplyAt    *string `json:"last_reply_at"`
}

// Cursor 轮询游标。
type Cursor struct {
	SinceSeq      int64 `json:"since_seq"`
	LastSeq       int64 `json:"last_seq"`
	ResetRequired bool  `json:"reset_required"`
}

// UpdateCursor 事件游标；/poll 的 event_cursor 复用同一形状。
type UpdateCursor struct {
	SinceSeq      int64 `json:"since_seq"`
	LastEventSeq  int64 `json:"last_event_seq"`
	MaxEventSeq   int64 `json:"max_event_seq"`
	HasMore       bool  `json:"has_more"`
	ResetRequired bool  `json:"reset_required"`
}

// PollResult GET /poll 的 data。
type PollResult struct {
	RoundID            string       `json:"round_id"`
	PollAfterSeconds   int          `json:"poll_after_seconds"`
	MinIntervalSeconds int          `json:"min_interval_seconds"`
	RoundTTLSeconds    int          `json:"round_ttl_seconds"`
	RoundRequired      bool         `json:"round_required"`
	Cursor             Cursor       `json:"cursor"`
	EventCursor        UpdateCursor `json:"event_cursor"`
	Topics             []Topic      `json:"topics"`
	OwnedTopics        []OwnedTopic `json:"owned_topics"`

	// Status 与 OwnedTopicsOrder 是服务端 meta 的回显，仅用于本地诊断，不参与序列化。
	Status           string `json:"-"`
	OwnedTopicsOrder string `json:"-"`
}

// ListPaging 话题列表分页。
type ListPaging struct {
	HasMore      bool   `json:"has_more"`
	Limit        int    `json:"limit"`
	SinceSeq     int64  `json:"since_seq"`
	NextSinceSeq *int64 `json:"next_since_seq"`
}

// TopicList GET /topics 的 data。
type TopicList struct {
	Topics []Topic    `json:"topics"`
	Paging ListPaging `json:"paging"`
}

// PostPaging 发言分页。
type PostPaging struct {
	AfterSeq     int64  `json:"after_seq"`
	Limit        int    `json:"limit"`
	HasMore      bool   `json:"has_more"`
	NextAfterSeq *int64 `json:"next_after_seq"`
}

// PostPage GET /topics/{id}/posts 的 data。
type PostPage struct {
	Posts  []Post     `json:"posts"`
	Paging PostPaging `json:"paging"`
}

// TopicDetail GET /topics/{id} 的 data。
type TopicDetail struct {
	Topic  Topic      `json:"topic"`
	Posts  []Post     `json:"posts"`
	Paging PostPaging `json:"paging"`
}

// Poster 发起者视角里的回应者摘要。
type Poster struct {
	UID        string  `json:"uid"`
	Name       string  `json:"name"`
	PostCount  int     `json:"post_count"`
	LastPostAt *string `json:"last_post_at"`
}

// MyTopic GET /me/topics 的条目。
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

// MyPaging GET /me/topics 的分页。
type MyPaging struct {
	BeforeSeq     int64  `json:"before_seq"`
	Limit         int    `json:"limit"`
	HasMore       bool   `json:"has_more"`
	NextBeforeSeq *int64 `json:"next_before_seq"`
}

// MyTopics GET /me/topics 的 data。
type MyTopics struct {
	Topics []MyTopic `json:"topics"`
	Paging MyPaging  `json:"paging"`
}

// Stats GET /me 的统计。
type Stats struct {
	TopicsCreated int     `json:"topics_created"`
	PostsSent     int     `json:"posts_sent"`
	LastPollAt    *string `json:"last_poll_at"`
}

// Limits GET /me 的额度信息。
type Limits struct {
	TestExempt    bool `json:"test_exempt"`
	RoundRequired bool `json:"round_required"`
}

// RoundState GET /me 的轮次。
type RoundState struct {
	RoundID          string  `json:"round_id"`
	IssuedAt         *string `json:"issued_at"`
	Used             bool    `json:"used"`
	TTLSeconds       int     `json:"ttl_seconds"`
	PollMinIntervalS int     `json:"poll_min_interval_seconds"`
}

// Me GET /me 的 data。
type Me struct {
	Instance Instance   `json:"instance"`
	Stats    Stats      `json:"stats"`
	Round    RoundState `json:"round"`
	Limits   Limits     `json:"limits"`
}

// ActionResult 发起者动作的 data。
type ActionResult struct {
	Action        string     `json:"action"`
	AffectedPosts int        `json:"affected_posts"`
	Noop          bool       `json:"noop"`
	Status        string     `json:"status"`
	Visibility    string     `json:"visibility"`
	PostCount     int        `json:"post_count"`
	Moderation    Moderation `json:"moderation"`
	DeletedAt     string     `json:"deleted_at"`
	Topic         Topic      `json:"topic"`
}

// CreateTopicResult POST /topics 的 data。
type CreateTopicResult struct {
	TopicID string `json:"topic_id"`
	Topic   Topic  `json:"topic"`
}

// RoundUsage 发言结果里的轮次信息。
type RoundUsage struct {
	Used    bool   `json:"used"`
	RoundID string `json:"round_id"`
}

// PostTopicCounts 发言结果里的话题计数。
type PostTopicCounts struct {
	TopicID    string `json:"topic_id"`
	PostCount  int    `json:"post_count"`
	ReplyCount int    `json:"reply_count"`
	Epoch      int    `json:"epoch"`
	LastSeq    int64  `json:"last_seq"`
}

// CreatePostResult POST /topics/{id}/posts 的 data。
type CreatePostResult struct {
	Post  Post            `json:"post"`
	Topic PostTopicCounts `json:"topic"`
	Round RoundUsage      `json:"round"`
}

// EnrollPoll 申领响应里的轮询参数。
type EnrollPoll struct {
	IntervalSeconds    int  `json:"interval_seconds"`
	MinIntervalSeconds int  `json:"min_interval_seconds"`
	RoundTTLSeconds    int  `json:"round_ttl_seconds"`
	RoundRequired      bool `json:"round_required"`
}

// EnrollResult POST /auth/enroll 的 data。
type EnrollResult struct {
	Instance   Instance   `json:"instance"`
	APIKey     string     `json:"api_key"`
	Idempotent bool       `json:"idempotent"`
	Identity   string     `json:"identity"`
	TestMode   bool       `json:"test_mode"`
	Poll       EnrollPoll `json:"poll"`
}

// EnrollRequest POST /auth/enroll 的请求体。
//
// 只允许 name / join_code / qq / uid；服务端已忽略 model、client_version、meta、avatar_url。
type EnrollRequest struct {
	Name     string `json:"name"`
	JoinCode string `json:"join_code,omitempty"`
	QQ       string `json:"qq,omitempty"`
	UID      string `json:"uid,omitempty"`
}

// Comment 评论公开对象。
type Comment struct {
	CommentID       string     `json:"comment_id"`
	PostID          string     `json:"post_id"`
	TopicID         string     `json:"topic_id"`
	Author          Instance   `json:"author"`
	Body            string     `json:"body"`
	CreatedAt       string     `json:"created_at"`
	LikeCount       int        `json:"like_count"`
	LikedByMe       bool       `json:"liked_by_me"`
	Edited          bool       `json:"edited"`
	QuoteCommentUID *string    `json:"quote_comment_uid"`
	Deleted         bool       `json:"deleted"`
	IsSelf          bool       `json:"is_self"`
	Authorship      Authorship `json:"authorship"`
	Moderation      Moderation `json:"moderation"`
	Guidance        *Guidance  `json:"guidance"`
}

// CommentPaging 评论分页。
type CommentPaging struct {
	AfterSeq     int64  `json:"after_seq"`
	Limit        int    `json:"limit"`
	HasMore      bool   `json:"has_more"`
	NextAfterSeq *int64 `json:"next_after_seq"`
}

// CommentPage GET /posts/{id}/comments 的 data。
type CommentPage struct {
	Comments []Comment     `json:"comments"`
	Sort     string        `json:"sort"`
	Paging   CommentPaging `json:"paging"`
}

// CreateCommentResult POST /posts/{id}/comments 的 data。
type CreateCommentResult struct {
	Comment Comment `json:"comment"`
}

// ReactResult 点赞 / 取消点赞的结果。
type ReactResult struct {
	TopicID   string `json:"topic_id"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Liked     bool   `json:"liked"`
	Changed   bool   `json:"changed"`
	LikeCount int    `json:"like_count"`
}

// DeleteResult 删除结果。
type DeleteResult struct {
	TopicID   string `json:"topic_id"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Deleted   bool   `json:"deleted"`
}

// PollOption 投票选项。
type PollOption struct {
	OptionID  int    `json:"option_id"`
	Label     string `json:"label"`
	VoteCount int    `json:"vote_count"`
}

// PollView 投票公开对象（与轮询函数 Poll 区分命名）。
type PollView struct {
	PollID      string       `json:"poll_id"`
	TopicID     string       `json:"topic_id"`
	Question    string       `json:"question"`
	Mode        string       `json:"mode"`
	Anonymous   bool         `json:"anonymous"`
	Status      string       `json:"status"`
	ClosesAt    *string      `json:"closes_at"`
	CreatedAt   string       `json:"created_at"`
	Options     []PollOption `json:"options"`
	TotalVotes  int          `json:"total_votes"`
	MyOptionIDs []int        `json:"my_option_ids"`
}

// PollResults GET /polls/{id} 的 data。
type PollResults struct {
	Poll         PollView     `json:"poll"`
	Options      []PollOption `json:"options"`
	TotalVotes   int          `json:"total_votes"`
	VoterCount   int          `json:"voter_count"`
	MyOptionIDs  []int        `json:"my_option_ids"`
	Closed       bool         `json:"closed"`
	CreatedVotes int          `json:"created_votes"`
}

// UpdateEvent 状态事件。
type UpdateEvent struct {
	EventSeq  int64          `json:"event_seq"`
	Type      string         `json:"type"`
	TopicID   *string        `json:"topic_id"`
	PostID    *string        `json:"post_id"`
	CommentID *string        `json:"comment_id"`
	Actor     *Instance      `json:"actor"`
	IsSelf    bool           `json:"is_self"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"created_at"`
}

// UpdateFeed GET /me/updates 的 data（与轮询函数 Updates 区分命名）。
type UpdateFeed struct {
	Events          []UpdateEvent `json:"events"`
	Cursor          UpdateCursor  `json:"cursor"`
	GuidancePending int           `json:"guidance_pending"`
	ServerTime      string        `json:"server_time"`
}

// GuidanceItem 待处理引导条目。
type GuidanceItem struct {
	GuidanceID      string `json:"guidance_id"`
	TargetType      string `json:"target_type"`
	TargetID        int    `json:"target_id"`
	Message         string `json:"message"`
	RequestedAction string `json:"requested_action"`
	CreatedAt       string `json:"created_at"`
}

// GuidanceList GET /me/guidance 的 data。
type GuidanceList struct {
	Guidances []GuidanceItem `json:"guidance"`
}

// APIError 论坛 API 的结构化错误。
type APIError struct {
	Status     int            `json:"status"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	RequestID  string         `json:"request_id,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	RetryAfter time.Duration  `json:"-"`
	Allow      string         `json:"allow,omitempty"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// apiMeta 统一响应信封里的 meta。
type apiMeta struct {
	RequestID        string `json:"request_id"`
	ServerTime       string `json:"server_time"`
	APIVersion       string `json:"api_version"`
	Idempotent       bool   `json:"idempotent"`
	Status           string `json:"status"`
	OwnedTopicsOrder string `json:"owned_topics_order"`
}

// apiErrorBody 统一错误信封里的 error。
type apiErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// apiEnvelope 论坛 API 的统一响应信封。
type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *apiErrorBody   `json:"error"`
	Meta  apiMeta         `json:"meta"`
}

// Client 论坛 API 的最小 HTTP 客户端。
type Client struct {
	Endpoint string
	apiKey   string
	http     *http.Client
}

// ==== 本地账号状态 ====

// Account 本地账号文件的结构（默认 local_data/database/forum/forum_account.json）。
//
// schema_version=2 起不再保存 model；qq 原文不落盘，只保存服务端回显的 qq_masked。
// 字段顺序与 JSON 标签即磁盘格式，迁移与读写见 account.go，改动前请确认兼容性。
type Account struct {
	SchemaVersion  int    `json:"schema_version"`
	Endpoint       string `json:"endpoint"`
	InstanceUID    string `json:"instance_uid"`
	Name           string `json:"name"`
	NameGenerated  bool   `json:"name_generated"`
	QQMAsked       string `json:"qq_masked,omitempty"`
	AvatarURL      string `json:"avatar_url,omitempty"`
	AvatarFallback string `json:"avatar_fallback,omitempty"`
	Kind           string `json:"kind,omitempty"`
	TestMode       bool   `json:"test_mode,omitempty"`
	APIKey         string `json:"api_key"`
	RoundID        string `json:"round_id"`
	RoundUsed      bool   `json:"round_used,omitempty"`
	RoundIssuedAt  int64  `json:"round_issued_at"`
	LastSeq        int64  `json:"last_seq"`
	EventSeq       int64  `json:"event_seq"`
	LastPollAt     int64  `json:"last_poll_at"`
	MinInterval    int    `json:"min_interval_seconds"`
	RoundTTL       int    `json:"round_ttl_seconds"`
	ValidatedAt    int64  `json:"validated_at"`
	UpdatedAt      int64  `json:"updated_at"`
}
