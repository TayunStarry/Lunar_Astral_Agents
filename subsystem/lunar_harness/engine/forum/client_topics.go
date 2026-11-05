package forum

// client_topics.go 论坛话题与轮次的 HTTP 端点。

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Enroll 申领或恢复身份。
func (c *Client) Enroll(ctx context.Context, req EnrollRequest) (*EnrollResult, error) {
	var out EnrollResult
	if _, err := c.request(ctx, http.MethodPost, "/auth/enroll", nil, req, false, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Poll 领取轮次、话题流与我方话题摘要。
func (c *Client) Poll(ctx context.Context, sinceSeq int64, status string) (*PollResult, error) {
	query := url.Values{}
	if sinceSeq > 0 {
		query.Set("since_seq", fmt.Sprintf("%d", sinceSeq))
	}
	if status != "" {
		query.Set("status", status)
	}
	var out PollResult
	meta, err := c.request(ctx, http.MethodGet, "/poll", query, nil, true, &out)
	if err != nil {
		return nil, err
	}
	out.Status = meta.Status
	out.OwnedTopicsOrder = meta.OwnedTopicsOrder
	return &out, nil
}

// ListTopics 按状态分页列出话题。
func (c *Client) ListTopics(ctx context.Context, status string, sinceSeq int64, limit int) (*TopicList, error) {
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	if sinceSeq > 0 {
		query.Set("since_seq", fmt.Sprintf("%d", sinceSeq))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out TopicList
	if _, err := c.request(ctx, http.MethodGet, "/topics", query, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListMyTopics 分页列出发起者自己的话题。
func (c *Client) ListMyTopics(ctx context.Context, status string, beforeSeq int64, limit int) (*MyTopics, error) {
	query := url.Values{}
	if status != "" {
		query.Set("status", status)
	}
	if beforeSeq > 0 {
		query.Set("before_seq", fmt.Sprintf("%d", beforeSeq))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out MyTopics
	if _, err := c.request(ctx, http.MethodGet, "/me/topics", query, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Topic 读取话题详情与发言分页。
func (c *Client) Topic(ctx context.Context, topicID string, afterSeq int64, limit int) (*TopicDetail, error) {
	query := url.Values{}
	if afterSeq > 0 {
		query.Set("after_seq", fmt.Sprintf("%d", afterSeq))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out TopicDetail
	if _, err := c.request(ctx, http.MethodGet, "/topics/"+topicID, query, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Posts 只读取某话题的发言分页。
func (c *Client) Posts(ctx context.Context, topicID string, afterSeq int64, limit int) (*PostPage, error) {
	query := url.Values{}
	if afterSeq > 0 {
		query.Set("after_seq", fmt.Sprintf("%d", afterSeq))
	}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	var out PostPage
	if _, err := c.request(ctx, http.MethodGet, "/topics/"+topicID+"/posts", query, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateTopic 发起话题。
func (c *Client) CreateTopic(ctx context.Context, title, body, requestID string) (*CreateTopicResult, error) {
	payload := map[string]any{
		"title":             title,
		"body":              body,
		"client_request_id": requestID,
	}
	var out CreateTopicResult
	if _, err := c.request(ctx, http.MethodPost, "/topics", nil, payload, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreatePost 在话题下发言。
func (c *Client) CreatePost(ctx context.Context, topicID, body, roundID, requestID string) (*CreatePostResult, error) {
	payload := map[string]any{
		"body":              body,
		"client_request_id": requestID,
	}
	if roundID != "" {
		payload["round_id"] = roundID
	}
	var out CreatePostResult
	if _, err := c.request(ctx, http.MethodPost, "/topics/"+topicID+"/posts", nil, payload, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ManageTopic 执行 reset / clear / complete / close。
func (c *Client) ManageTopic(ctx context.Context, topicID, action string) (*ActionResult, error) {
	var out ActionResult
	if _, err := c.request(ctx, http.MethodPost, "/topics/"+topicID+"/actions/"+action, nil, nil, true, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
