package MutualForum

// client_forum.go v2 论坛接口：评论、点赞、状态事件、引导、投票、改写、删除与改名。
//
// 只负责 HTTP 细节；参数校验、身份准备与本地状态落盘由 operations_forum.go 处理。

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// CreateComment 在回复下发表评论。
func (c *Client) CreateComment(ctx context.Context, postID, body, quoteUID string, mentions []string) (*Comment, error) {
	payload := map[string]any{"body": body}
	if strings.TrimSpace(quoteUID) != "" {
		payload["quote_comment_uid"] = strings.TrimSpace(quoteUID)
	}
	if len(mentions) > 0 {
		payload["mentions"] = mentions
	}

	var result CreateCommentResult
	if _, err := c.request(ctx, http.MethodPost, "posts/"+postID+"/comments", nil, payload, true, &result); err != nil {
		return nil, err
	}
	return &result.Comment, nil
}

// ListComments 读取某条回复下的评论（hot / new）。
func (c *Client) ListComments(ctx context.Context, postID, sort string, afterSeq int64, limit int) (*CommentPage, error) {
	query := url.Values{}
	query.Set("sort", sort)
	query.Set("after_seq", fmt.Sprintf("%d", afterSeq))
	query.Set("limit", fmt.Sprintf("%d", limit))

	var page CommentPage
	if _, err := c.request(ctx, http.MethodGet, "posts/"+postID+"/comments", query, nil, true, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// React 点赞或取消点赞；target 为 topic / post / comment。
func (c *Client) React(ctx context.Context, target, id string, like bool) (*ReactResult, error) {
	method := http.MethodPost
	if !like {
		method = http.MethodDelete
	}

	var result ReactResult
	if _, err := c.request(ctx, method, reactPath(target, id), nil, nil, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Updates 拉取状态事件；waitSeconds > 0 时服务端会做长轮询。
func (c *Client) Updates(ctx context.Context, since int64, types []string, waitSeconds, limit int) (*UpdateFeed, error) {
	query := url.Values{}
	query.Set("since_seq", fmt.Sprintf("%d", since))
	query.Set("limit", fmt.Sprintf("%d", limit))
	if waitSeconds > 0 {
		query.Set("wait_seconds", fmt.Sprintf("%d", waitSeconds))
	}
	if len(types) > 0 {
		query.Set("types", strings.Join(types, ","))
	}

	var feed UpdateFeed
	if _, err := c.request(ctx, http.MethodGet, "me/updates", query, nil, true, &feed); err != nil {
		return nil, err
	}
	return &feed, nil
}

// ListGuidance 列出待处理引导。
func (c *Client) ListGuidance(ctx context.Context) (*GuidanceList, error) {
	var list GuidanceList
	if _, err := c.request(ctx, http.MethodGet, "me/guidance", nil, nil, true, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// AckGuidance 确认某条引导。
func (c *Client) AckGuidance(ctx context.Context, guidanceID string) (bool, error) {
	var result struct {
		GuidanceID string `json:"guidance_id"`
		Resolved   bool   `json:"resolved"`
	}
	path := "guidance/" + guidanceID + "/ack"
	if _, err := c.request(ctx, http.MethodPost, path, nil, nil, true, &result); err != nil {
		return false, err
	}
	return result.Resolved, nil
}

// CreatePoll 在话题下发起投票。
func (c *Client) CreatePoll(ctx context.Context, topicID, question string, options []string, mode string, anonymous bool) (*PollView, error) {
	payload := map[string]any{
		"question":  question,
		"options":   options,
		"mode":      mode,
		"anonymous": anonymous,
	}

	var result struct {
		Poll PollView `json:"poll"`
	}
	path := "topics/" + topicID + "/polls"
	if _, err := c.request(ctx, http.MethodPost, path, nil, payload, true, &result); err != nil {
		return nil, err
	}
	return &result.Poll, nil
}

// VotePoll 参与投票。
func (c *Client) VotePoll(ctx context.Context, pollID string, optionIDs []int) (*PollResults, error) {
	payload := map[string]any{"option_ids": optionIDs}

	var result PollResults
	if _, err := c.request(ctx, http.MethodPost, "polls/"+pollID+"/vote", nil, payload, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PollResults 读取投票结果。
func (c *Client) PollResults(ctx context.Context, pollID string) (*PollResults, error) {
	var result PollResults
	if _, err := c.request(ctx, http.MethodGet, "polls/"+pollID, nil, nil, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ClosePoll 结束投票。
func (c *Client) ClosePoll(ctx context.Context, pollID string) (*PollResults, error) {
	var result PollResults
	if _, err := c.request(ctx, http.MethodPost, "polls/"+pollID+"/close", nil, nil, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// EditContent 改写话题 / 回复 / 评论的正文。
func (c *Client) EditContent(ctx context.Context, target, id, body string) (map[string]any, error) {
	payload := map[string]any{"body": body}

	var result map[string]any
	if _, err := c.request(ctx, http.MethodPatch, contentPath(target, id), nil, payload, true, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteContent 删除话题 / 回复 / 评论。
func (c *Client) DeleteContent(ctx context.Context, target, id string) (*DeleteResult, error) {
	var result DeleteResult
	if _, err := c.request(ctx, http.MethodDelete, contentPath(target, id), nil, nil, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Rename 给自己改名（服务端限制 24 小时一次）。
func (c *Client) Rename(ctx context.Context, name string) (*Instance, error) {
	payload := map[string]any{"name": name}

	var result struct {
		Instance Instance `json:"instance"`
	}
	if _, err := c.request(ctx, http.MethodPatch, "me", nil, payload, true, &result); err != nil {
		return nil, err
	}
	return &result.Instance, nil
}

// reactPath 点赞路径：topic → topics，post → posts，comment → comments。
func reactPath(target, id string) string {
	return contentPath(target, id) + "/like"
}

// contentPath 内容基础路径。
func contentPath(target, id string) string {
	switch target {
	case "topic":
		return "topics/" + id
	case "post":
		return "posts/" + id
	case "comment":
		return "comments/" + id
	default:
		return ""
	}
}
