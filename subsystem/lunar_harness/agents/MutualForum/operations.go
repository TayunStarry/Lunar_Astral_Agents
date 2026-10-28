package MutualForum

// operations.go 论坛原生能力的桥接：轮询、话题、发言、发起者动作与本地检索。
//
// 所有导出函数都自行读取独立配置、校验开关与地址，并在需要身份时复用/申领账号。
// 调用方（agents/MutualForum）只需要表达业务意图，不必关心 HTTP 细节。

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var topicIDPattern = regexp.MustCompile(`^t_[0-9a-f]{8,32}$`)

// clientWithConfig 读取配置并构造客户端；未启用或地址无效时直接返回错误。
func clientWithConfig() (Config, *Client, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return cfg, nil, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, nil, err
	}
	return cfg, newClient(cfg), nil
}

// signedIn 确保客户端带有可用身份。
func signedIn(ctx context.Context, cfg Config, c *Client) (*Account, error) {
	acct, err := c.EnsureAccount(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return acct, nil
}

// CheckHealth 返回论坛健康状态（不需要身份）。
func CheckHealth(ctx context.Context) (*Health, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, cfg.HealthTimeout())
	defer cancel()
	return c.Health(probeCtx)
}

// Identity 读取当前账号的身份、统计与轮次状态。
func Identity(ctx context.Context) (*Me, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.Me(ctx)
}

/* ---------------- 轮询与轮次 ---------------- */

// Poll 领取新轮次，并返回可参与的话题与自己的话题摘要。
func Poll(ctx context.Context, sinceSeq int64) (*PollResult, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	acct, err := signedIn(ctx, cfg, c)
	if err != nil {
		return nil, err
	}
	return pollWithAccount(ctx, cfg, c, acct, sinceSeq)
}

// pollWithAccount 在已有账号状态上执行轮询：遵守最小间隔、处理 429、落盘新轮次。
func pollWithAccount(ctx context.Context, cfg Config, c *Client, acct *Account, sinceSeq int64) (*PollResult, error) {
	if sinceSeq < 0 {
		sinceSeq = 0
	}
	minInterval := acct.MinInterval
	if minInterval <= 0 {
		minInterval = cfg.PollMinInterval()
	}
	if acct.LastPollAt > 0 {
		wait := time.Duration(minInterval)*time.Second - time.Since(time.Unix(acct.LastPollAt, 0))
		if wait > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	query := url.Values{}
	query.Set("since_seq", fmt.Sprintf("%d", sinceSeq))

	var result PollResult
	for attempt := 0; ; attempt++ {
		_, err := c.request(ctx, http.MethodGet, "poll", query, nil, true, &result)
		if err == nil {
			break
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != "poll_too_frequent" || attempt >= maxPollRetries {
			return nil, err
		}
		wait := time.Duration(apiErr.RetryAfter) * time.Second
		if wait <= 0 {
			wait = time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	now := time.Now().Unix()
	acct.RoundID = result.RoundID
	acct.RoundIssuedAt = now
	acct.LastPollAt = now
	acct.LastSeq = result.Cursor.LastSeq
	if result.MinIntervalSeconds > 0 {
		acct.MinInterval = result.MinIntervalSeconds
	}
	if result.RoundTTLSeconds > 0 {
		acct.RoundTTL = result.RoundTTLSeconds
	}
	if err := saveAccount(cfg, acct); err != nil {
		return nil, err
	}
	return &result, nil
}

// EnsureRound 返回当前可用轮次；没有或临近过期时自动轮询续领。
func EnsureRound(ctx context.Context, cfg Config, c *Client, acct *Account) (string, error) {
	ttl := acct.RoundTTL
	if ttl <= 0 {
		ttl = cfg.RoundTTL()
	}
	if acct.RoundID != "" && acct.RoundIssuedAt > 0 {
		age := time.Now().Unix() - acct.RoundIssuedAt
		if age < int64(ttl-roundRefreshGuard) {
			return acct.RoundID, nil
		}
	}
	result, err := pollWithAccount(ctx, cfg, c, acct, acct.LastSeq)
	if err != nil {
		return "", err
	}
	return result.RoundID, nil
}

/* ---------------- 话题与发言读取 ---------------- */

// ListTopics 按状态列出话题。
func ListTopics(ctx context.Context, status string, sinceSeq int64, limit int) (*TopicList, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	status = normalizeStatus(status)
	limit = clampInt(limit, 1, 50, 20)

	query := url.Values{}
	query.Set("status", status)
	query.Set("since_seq", fmt.Sprintf("%d", maxInt64(sinceSeq, 0)))
	query.Set("limit", fmt.Sprintf("%d", limit))

	var list TopicList
	if _, err := c.request(ctx, http.MethodGet, "topics", query, nil, true, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ListMyTopics 列出发起者自己的话题与回应摘要。
func ListMyTopics(ctx context.Context, status string, limit int) (*MyTopics, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("status", normalizeStatus(status))
	query.Set("limit", fmt.Sprintf("%d", clampInt(limit, 1, 50, 50)))

	var mine MyTopics
	if _, err := c.request(ctx, http.MethodGet, "me/topics", query, nil, true, &mine); err != nil {
		return nil, err
	}
	return &mine, nil
}

// GetTopic 读取话题详情与发言分页。
func GetTopic(ctx context.Context, topicID string, afterSeq int64, limit int) (*TopicDetail, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("after_seq", fmt.Sprintf("%d", maxInt64(afterSeq, 0)))
	query.Set("limit", fmt.Sprintf("%d", clampInt(limit, 1, 100, 50)))

	var detail TopicDetail
	if _, err := c.request(ctx, http.MethodGet, "topics/"+topicID, query, nil, true, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// ListPosts 只读取某话题的发言分页。
func ListPosts(ctx context.Context, topicID string, afterSeq int64, limit int) (*PostPage, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Set("after_seq", fmt.Sprintf("%d", maxInt64(afterSeq, 0)))
	query.Set("limit", fmt.Sprintf("%d", clampInt(limit, 1, 100, 100)))

	var page PostPage
	if _, err := c.request(ctx, http.MethodGet, "topics/"+topicID+"/posts", query, nil, true, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

/* ---------------- 写操作 ---------------- */

// CreateTopic 发起求助话题。
func CreateTopic(ctx context.Context, title, body, requestID string) (*CreateTopicResult, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" || body == "" {
		return nil, errors.New("话题标题和正文都不能为空")
	}
	payload := map[string]any{"title": title, "body": body}
	if requestID = strings.TrimSpace(requestID); requestID == "" {
		requestID = newRequestID("lm-topic")
	}
	payload["client_request_id"] = requestID

	var result CreateTopicResult
	if _, err := c.request(ctx, http.MethodPost, "topics", nil, payload, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Reply 在指定话题发言。
//
// 服务端「一轮一条」：内部自动领取轮次并在成功后清空本地轮次；
// 旧轮次失效或过期时自动重新轮询并重试一次。
func Reply(ctx context.Context, topicID, body, requestID string) (*CreatePostResult, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("发言内容不能为空")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	acct, err := signedIn(ctx, cfg, c)
	if err != nil {
		return nil, err
	}
	roundID, err := EnsureRound(ctx, cfg, c, acct)
	if err != nil {
		return nil, err
	}

	payload := func(round string) map[string]any {
		key := strings.TrimSpace(requestID)
		if key == "" {
			key = newRequestID("lm-reply")
		}
		return map[string]any{"body": body, "round_id": round, "client_request_id": key}
	}

	var result CreatePostResult
	_, err = c.request(ctx, http.MethodPost, "topics/"+topicID+"/posts", nil, payload(roundID), true, &result)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && (apiErr.Code == "round_superseded" || apiErr.Code == "round_expired") {
			acct.RoundID = ""
			acct.RoundIssuedAt = 0
			newRound, roundErr := EnsureRound(ctx, cfg, c, acct)
			if roundErr != nil {
				return nil, roundErr
			}
			_, retryErr := c.request(ctx, http.MethodPost, "topics/"+topicID+"/posts", nil, payload(newRound), true, &result)
			if retryErr != nil {
				return nil, retryErr
			}
		} else {
			return nil, err
		}
	}

	acct.RoundID = ""
	acct.RoundIssuedAt = 0
	if err := saveAccount(cfg, acct); err != nil {
		return nil, err
	}
	return &result, nil
}

// ManageTopic 执行发起者动作：reset / clear / complete / close。
func ManageTopic(ctx context.Context, topicID, action string) (*ActionResult, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "reset", "clear", "complete", "close":
	default:
		return nil, fmt.Errorf("不支持的话题动作 %q（只允许 reset / clear / complete / close）", action)
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	var result ActionResult
	if _, err := c.request(ctx, http.MethodPost, "topics/"+topicID+"/actions/"+action, nil, nil, true, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

/* ---------------- 本地检索 ---------------- */

// Search 在话题标题、正文与（可选）发言中做关键词检索。
//
// 论坛本身没有搜索端点，这里用列表与详情接口在客户端完成检索与排序，
// 让月华可以先搜索、再决定回应哪个话题。
func Search(ctx context.Context, query, status, scope string, limit int) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("搜索关键词不能为空")
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = cfg.SearchSize()
	}
	limit = clampInt(limit, 1, maxSearchLimit, cfg.SearchSize())
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		scope = "topics"
	}
	if scope != "topics" && scope != "posts" && scope != "all" {
		return nil, errors.New("scope 只允许 topics / posts / all")
	}

	list, err := ListTopics(ctx, status, 0, 50)
	if err != nil {
		return nil, err
	}

	terms := searchTerms(query)
	hitMap := map[string]*SearchHit{}
	order := make([]string, 0)
	for _, topic := range list.Topics {
		score, matched := scoreText(topic.Title, topic.Body, terms)
		if score <= 0 {
			continue
		}
		hitMap[topic.TopicID] = &SearchHit{Topic: topic, Score: score, MatchedIn: matched}
		order = append(order, topic.TopicID)
	}

	if scope == "posts" || scope == "all" {
		// 发言检索：按「标题正文命中优先、其余按最近更新」取有限候选，避免逐条扫全站。
		candidates := append([]Topic(nil), list.Topics...)
		sort.SliceStable(candidates, func(i, j int) bool {
			scoreI, _ := scoreText(candidates[i].Title, candidates[i].Body, terms)
			scoreJ, _ := scoreText(candidates[j].Title, candidates[j].Body, terms)
			if scoreI != scoreJ {
				return scoreI > scoreJ
			}
			return candidates[i].LastSeq > candidates[j].LastSeq
		})
		const maxPostCandidates = 10
		if len(candidates) > maxPostCandidates {
			candidates = candidates[:maxPostCandidates]
		}
		for _, topic := range candidates {
			page, pageErr := ListPosts(ctx, topic.TopicID, 0, 100)
			if pageErr != nil {
				continue
			}
			hit := hitMap[topic.TopicID]
			for _, post := range page.Posts {
				postScore, _ := scoreText("", post.Body, terms)
				if postScore <= 0 {
					continue
				}
				if hit == nil {
					hit = &SearchHit{Topic: topic}
					hitMap[topic.TopicID] = hit
					order = append(order, topic.TopicID)
				}
				hit.Score += postScore
				hit.MatchedIn = appendUnique(hit.MatchedIn, fmt.Sprintf("post#%d", post.Seq))
				hit.Posts = append(hit.Posts, post)
			}
		}
	}

	hits := make([]SearchHit, 0, len(order))
	for _, topicID := range order {
		if hit := hitMap[topicID]; hit != nil {
			hits = append(hits, *hit)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Topic.LastSeq > hits[j].Topic.LastSeq
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// searchTerms 把查询拆成整句 + 空白分词，兼容中文与英文关键词。
func searchTerms(query string) []string {
	terms := []string{strings.ToLower(query)}
	for _, part := range strings.Fields(query) {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" && !containsString(terms, part) {
			terms = append(terms, part)
		}
	}
	return terms
}

// scoreText 统计关键词在标题/正文中的出现次数；标题权重更高。
func scoreText(title, body string, terms []string) (int, []string) {
	lowerTitle := strings.ToLower(title)
	lowerBody := strings.ToLower(body)
	score := 0
	matched := make([]string, 0, 2)
	for _, term := range terms {
		if term == "" {
			continue
		}
		if count := strings.Count(lowerTitle, term); count > 0 {
			score += count * 3
			matched = appendUnique(matched, "title")
		}
		if count := strings.Count(lowerBody, term); count > 0 {
			score += count
			matched = appendUnique(matched, "body")
		}
	}
	return score, matched
}

/* ---------------- 小工具 ---------------- */

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "", "all":
		return "all"
	case "open", "completed", "closed":
		return status
	default:
		return "all"
	}
}

func clampInt(value, low, high, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxInt64(value, fallback int64) int64 {
	if value < 0 {
		return fallback
	}
	return value
}

func validateTopicID(topicID string) error {
	topicID = strings.TrimSpace(topicID)
	if !topicIDPattern.MatchString(topicID) {
		return fmt.Errorf("话题编号格式不正确：%q（应形如 t_0123456789ab）", topicID)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if containsString(values, value) {
		return values
	}
	return append(values, value)
}
