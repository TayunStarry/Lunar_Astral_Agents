package MutualForum

// unread.go 拉取未读回应并加载回复所需的原始语境。
//
// 拉取复用 UpdateFeed 与账号里的事件游标（Updates 成功后推进 EventSeq）；
// 话题/发言/评论的读取复用既有桥接函数，不复制任何 HTTP 请求实现。

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// unreadReply 一条待回应的未读事件。
type unreadReply struct {
	EventSeq  int64
	Type      string
	TopicID   string
	PostID    string
	CommentID string
	ActorName string
	Preview   string
	CreatedAt string
}

// replyContext 生成回复所需的原始语境。
type replyContext struct {
	EventType   string
	TopicID     string
	TopicTitle  string
	TopicBody   string
	PostBody    string
	CommentBody string
	ActorName   string
	CreatedAt   string
}

// unreadBatch 一次未读事件拉取结果；Cursor 与账号由拉取方持有，供持久化后推进游标。
type unreadBatch struct {
	Items  []unreadReply
	Cursor UpdateCursor
	cfg    Config
	acct   *Account
}

// pullUnreadReplies 拉取当前账号事件游标之后的回应类事件，但不推进游标。
// 调用方必须先把返回条目持久化到本地重试队列，再调用 advanceEventCursor。
func pullUnreadReplies(ctx context.Context, since int64) (unreadBatch, error) {
	feed, cfg, acct, err := fetchUpdateFeed(ctx, since, unreadReplyTypes, 0, unreadFetchLimit)
	if err != nil {
		return unreadBatch{}, err
	}
	return unreadBatch{
		Items:  extractUnreadReplies(feed.Events),
		Cursor: feed.Cursor,
		cfg:    cfg,
		acct:   acct,
	}, nil
}

// extractUnreadReplies 过滤出需要自动回应的他人事件；缺少回复目标的忽略。
func extractUnreadReplies(events []UpdateEvent) []unreadReply {
	out := make([]unreadReply, 0, len(events))
	for _, event := range events {
		if event.IsSelf || !wantsAutoReply(event.Type) {
			continue
		}
		item := unreadReply{
			EventSeq:  event.EventSeq,
			Type:      event.Type,
			Preview:   payloadText(event.Payload, "preview", "body", "content"),
			CreatedAt: event.CreatedAt,
		}
		if event.TopicID != nil {
			item.TopicID = strings.TrimSpace(*event.TopicID)
		}
		if event.PostID != nil {
			item.PostID = strings.TrimSpace(*event.PostID)
		}
		if event.CommentID != nil {
			item.CommentID = strings.TrimSpace(*event.CommentID)
		}
		if event.Actor != nil {
			item.ActorName = strings.TrimSpace(event.Actor.Name)
		}
		if item.TopicID == "" && item.PostID == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// wantsAutoReply 判断事件类型是否属于需要回应的「回应类」事件。
func wantsAutoReply(eventType string) bool {
	switch eventType {
	case "comment_created", "post_created", "mention", "quote":
		return true
	default:
		return false
	}
}

// loadReplyContext 读取话题、被回应的发言与被回应的评论原文。
func loadReplyContext(ctx context.Context, item unreadReply) (replyContext, error) {
	if item.TopicID == "" {
		return replyContext{}, errors.New("回应消息缺少话题编号，无法加载语境")
	}
	detail, err := GetTopic(ctx, item.TopicID, 0, 100)
	if err != nil {
		return replyContext{}, fmt.Errorf("读取话题上下文失败: %w", err)
	}
	rc := replyContext{
		EventType:  item.Type,
		TopicID:    item.TopicID,
		TopicTitle: detail.Topic.Title,
		TopicBody:  detail.Topic.Body,
		ActorName:  item.ActorName,
		CreatedAt:  item.CreatedAt,
	}
	for _, post := range detail.Posts {
		if post.PostID == item.PostID {
			rc.PostBody = post.Body
			break
		}
	}
	if item.CommentID != "" && item.PostID != "" {
		if page, pageErr := ListComments(ctx, item.PostID, "new", 0, 100); pageErr == nil {
			for _, comment := range page.Comments {
				if comment.CommentID == item.CommentID {
					rc.CommentBody = comment.Body
					break
				}
			}
		}
	}
	// 详情接口未命中时用事件摘要兜底，保证仍能基于原始语境生成回复。
	if rc.PostBody == "" {
		rc.PostBody = item.Preview
	}
	if rc.CommentBody == "" {
		rc.CommentBody = item.Preview
	}
	if strings.TrimSpace(rc.TopicTitle+rc.TopicBody+rc.PostBody+rc.CommentBody) == "" {
		return replyContext{}, errors.New("回应消息缺少可用的原始语境")
	}
	return rc, nil
}

// payloadText 按候选键读取事件 payload 里的文本摘要。
func payloadText(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}
