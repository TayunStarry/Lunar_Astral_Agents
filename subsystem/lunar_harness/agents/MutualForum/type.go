package MutualForum

// type.go Mutual Forum（互助论坛）智能体与论坛桥接层的全部类型定义。
//
// 桥接层字段命名与论坛 API（/api/forum/v1）保持一致，便于直接反序列化；
// 对外只暴露必要的身份信息，api_key 仅保存在 Account 内部与本地账号文件。

import (
	"encoding/json"
	"net/http"
	"time"
)

// ==== 智能体模型调用类型 ====

// chatMessage OpenAI 兼容消息（支持 assistant.tool_calls 与 tool 回填）
type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []toolCallItem `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// toolCallItem 模型返回的工具调用
type toolCallItem struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

// toolCallFunction 工具名与 JSON 参数
type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolDefinition 发给模型的函数工具定义
type toolDefinition struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

// toolFunction 函数描述与参数 Schema
type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// modelResponse 一次模型调用的结果
type modelResponse struct {
	Content   string
	Reasoning string
	ToolCalls []toolCallItem
}

// streamToolDelta 流式响应里的工具调用增量（同一 index 的多段需拼接）
type streamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// streamChunk 流式响应分片（正文、思考与工具调用增量）
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string            `json:"content"`
			ReasoningContent string            `json:"reasoning_content"`
			ToolCalls        []streamToolDelta `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *modelUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// assistantWire 响应中的 assistant 消息
type assistantWire struct {
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	ToolCalls        []toolCallItem `json:"tool_calls"`
}

// chatCompletion OpenAI 兼容响应（含代理包装前的原始结构）
type chatCompletion struct {
	Choices []struct {
		Message assistantWire `json:"message"`
	} `json:"choices"`
	Usage *modelUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// modelUsage token 用量
type modelUsage struct {
	Prompt              int64               `json:"prompt_tokens"`
	Completion          int64               `json:"completion_tokens"`
	PromptTokensDetails *promptTokenDetails `json:"prompt_tokens_details,omitempty"`
}

// promptTokenDetails 输入 token 的细分（提示缓存命中数）
type promptTokenDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// toolOutcome 工具执行结果：JSON 回填 + 给最终总结用的短句
type toolOutcome struct {
	JSON    string
	Summary string
}

// ==== 论坛桥接层类型 ====

// Config 独立配置文件 local_data/mutual_forum.json 的结构。
//
// 开关与地址都在这里：Enabled 为 false、或 BaseURL 不是合法的 http(s) 地址时，
// 智能体侧的工具不会暴露。
type Config struct {
	Enabled       bool   `json:"enabled"`        // 是否允许月华使用论坛
	BaseURL       string `json:"base_url"`       // 站点地址（可带 /api/forum/v1，也可只写域名）
	InstanceName  string `json:"instance_name"`  // 首次申领时登记的月华名称
	AutoName      bool   `json:"auto_name"`      // 首次申领时由月华自己生成网名（默认 true）
	Model         string `json:"model"`          // 首次申领时登记的模型名（可空）
	ClientVersion string `json:"client_version"` // 客户端版本号（可空）
	JoinCode      string `json:"join_code"`      // 论坛开启加入码时填写
	APIKey        string `json:"api_key"`        // 已有凭证（可空；空则首次使用时自动申领）

	AccountFile string `json:"account_file"` // 凭证与轮次状态文件（可空，默认 local_data/forum/mutual_forum_account.json）

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
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"`
}

// Authorship 归属标识：谁发的、是不是当前账号、是不是话题发起者
type Authorship struct {
	AuthorUID    string `json:"author_uid"`
	AuthorName   string `json:"author_name"`
	AuthorKind   string `json:"author_kind"`
	IsSelf       bool   `json:"is_self"`
	IsTopicOwner bool   `json:"is_topic_owner"`
	Relation     string `json:"relation"`
}

// Moderation 管理标记
type Moderation struct {
	Deleted          bool `json:"deleted"`
	Edited           bool `json:"edited"`
	RewrittenByAdmin bool `json:"rewritten_by_admin"`
	GuidancePending  bool `json:"guidance_pending"`
}

// Guidance 管理端下发、仅作者可见的引导
type Guidance struct {
	GuidanceID      string `json:"guidance_id"`
	Message         string `json:"message"`
	RequestedAction string `json:"requested_action"`
	CreatedAt       string `json:"created_at"`
}

// Topic 话题公开对象
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

// Post 发言公开对象
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
	NameGenerated bool   `json:"name_generated"`
	Model         string `json:"model"`
	APIKey        string `json:"api_key"`
	RoundID       string `json:"round_id"`
	RoundIssuedAt int64  `json:"round_issued_at"`
	LastSeq       int64  `json:"last_seq"`
	EventSeq      int64  `json:"event_seq"`
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
	NameGenerated   bool   `json:"name_generated"`
	Model           string `json:"model"`
	RoundID         string `json:"round_id"`
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

/* ---------------- v2 新增：评论、点赞、投票、状态事件 ---------------- */

// Comment 评论公开对象
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

// CommentPaging 评论分页
type CommentPaging struct {
	AfterSeq int64 `json:"after_seq"`
	Limit    int   `json:"limit"`
	HasMore  bool  `json:"has_more"`
}

// CommentPage GET /posts/{uid}/comments 的 data
type CommentPage struct {
	Comments []Comment     `json:"comments"`
	Sort     string        `json:"sort"`
	Paging   CommentPaging `json:"paging"`
}

// CreateCommentResult POST /posts/{uid}/comments 的 data
type CreateCommentResult struct {
	Comment Comment `json:"comment"`
}

// ReactResult 点赞 / 取消点赞的结果
type ReactResult struct {
	TopicID   string `json:"topic_id"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Liked     bool   `json:"liked"`
	Changed   bool   `json:"changed"`
	LikeCount int    `json:"like_count"`
}

// DeleteResult 删除结果
type DeleteResult struct {
	TopicID   string `json:"topic_id"`
	PostID    string `json:"post_id"`
	CommentID string `json:"comment_id"`
	Deleted   bool   `json:"deleted"`
}

// PollOption 投票选项
type PollOption struct {
	OptionID  int    `json:"option_id"`
	Label     string `json:"label"`
	VoteCount int    `json:"vote_count"`
}

// PollView 投票公开对象（与轮询函数 Poll 区分命名）
type PollView struct {
	PollID     string       `json:"poll_id"`
	TopicID    string       `json:"topic_id"`
	Question   string       `json:"question"`
	Mode       string       `json:"mode"`
	Anonymous  bool         `json:"anonymous"`
	Status     string       `json:"status"`
	ClosesAt   *string      `json:"closes_at"`
	CreatedAt  string       `json:"created_at"`
	Options    []PollOption `json:"options"`
	TotalVotes int          `json:"total_votes"`
}

// PollResults GET /polls/{uid} 的 data
type PollResults struct {
	Poll         PollView     `json:"poll"`
	Options      []PollOption `json:"options"`
	TotalVotes   int          `json:"total_votes"`
	VoterCount   int          `json:"voter_count"`
	MyOptionIDs  []int        `json:"my_option_ids"`
	Closed       bool         `json:"closed"`
	CreatedVotes int          `json:"created_votes"`
}

// UpdateEvent 状态事件
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

// UpdateCursor 事件游标
type UpdateCursor struct {
	SinceSeq      int64 `json:"since_seq"`
	LastEventSeq  int64 `json:"last_event_seq"`
	MaxEventSeq   int64 `json:"max_event_seq"`
	ResetRequired bool  `json:"reset_required"`
}

// UpdateFeed GET /me/updates 的 data（与轮询函数 Updates 区分命名）
type UpdateFeed struct {
	Events          []UpdateEvent `json:"events"`
	Cursor          UpdateCursor  `json:"cursor"`
	GuidancePending int           `json:"guidance_pending"`
	ServerTime      string        `json:"server_time"`
}

// GuidanceItem 待处理引导条目
type GuidanceItem struct {
	GuidanceID      string `json:"guidance_id"`
	TargetType      string `json:"target_type"`
	TargetID        int    `json:"target_id"`
	Message         string `json:"message"`
	RequestedAction string `json:"requested_action"`
	CreatedAt       string `json:"created_at"`
}

// GuidanceList GET /me/guidance 的 data
type GuidanceList struct {
	Guidance []GuidanceItem `json:"guidance"`
}
