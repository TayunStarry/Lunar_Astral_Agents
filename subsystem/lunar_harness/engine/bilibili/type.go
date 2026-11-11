package bilibili

// type.go B 站接口客户端的对外类型。
//
// 本包只负责接口协议：请求组装、参数与签名、分页、响应解包与字段映射。
// 选片、打分、审核、提示词与人设、入队与状态推进等业务判断留在 chain/viewer。

import "time"

// Fetcher 是已登录 B 站页面上下文里的请求通道。
//
// 页面请求必须走浏览器 fetch 才能自动携带 HttpOnly 登录 cookie，
// 生产实现是 chain/viewer 的浏览器会话，测试可替换为假实现。
type Fetcher interface {
	FetchText(method, target, contentType, body string, timeout time.Duration) (string, error)
	FetchForm(target string, fields map[string]string, timeout time.Duration) (string, error)
}

// Page 在 Fetcher 之上增加表单签名所需的 CSRF。
type Page interface {
	Fetcher
	// CSRFToken 返回 bili_jct（B 站表单接口的 csrf 参数）。
	CSRFToken() string
}

// Video 视频的基础信息（x/web-interface/view 的字段映射）。
type Video struct {
	Bvid  string
	Aid   int64
	Cid   int64
	Title string
	Owner string
	// OwnerMid 作者 UID（待看列表入库要求同时保留 UID 与昵称）
	OwnerMid    int64
	Duration    int
	URL         string
	Desc        string
	Pic         string
	PublishTime int64
	// 热度指标：参与候选综合匹配度加权
	View     int64
	Like     int64
	Coin     int64
	Favorite int64
	ReplyNum int64
	Danmaku  int64
}

// Comment 一条评论（视频印象、风险复核与回应确认共用）。
type Comment struct {
	User    string `json:"user"`
	Content string `json:"content"`
	Like    int    `json:"like"`
}

// MentionTarget 分享时 @ 的目标。
type MentionTarget struct {
	Name string `json:"name"`
	UID  int64  `json:"uid,omitempty"`
}

// InboxMessage 消息中心条目归一化后的结果（分类与去重属于业务判断，不在本包）。
type InboxMessage struct {
	Kind      string `json:"kind"`  // reply / at
	Venue     string `json:"venue"` // video / dynamic / article
	Type      int    `json:"type"`  // 1=视频，11=图片动态，12=专栏，17=纯文字动态
	Oid       string `json:"oid"`
	SourceID  string `json:"source_id,omitempty"`
	RootID    string `json:"root_id,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	MessageID string `json:"message_id,omitempty"`
	User      string `json:"user"`
	Mid       int64  `json:"mid,omitempty"`
	Message   string `json:"message"`
	MyContent string `json:"my_content,omitempty"`
	Title     string `json:"title,omitempty"`
	URI       string `json:"uri,omitempty"`
	Time      int64  `json:"time,omitempty"`
}

// ReplyRequest 发布一条评论/回复所需的参数。
//
// Root 与 Parent 同时为空表示发布根评论；Targets 非空时写入 at_name/at_mid 产生真实 @。
type ReplyRequest struct {
	Type    int
	Oid     string
	Message string
	Root    string
	Parent  string
	Targets []MentionTarget
}

// CommentPager 逐页读取评论。
//
// 页码推进与请求参数属于接口协议，由本包维护；何时停止翻页
// （去重后是否凑够条数、是否还要继续）由调用方决定。
type CommentPager struct {
	f      Fetcher
	typ    int
	oid    int64
	page   int
	ps     int
	byTime bool
}
