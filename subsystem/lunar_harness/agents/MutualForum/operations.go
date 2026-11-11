package MutualForum

// operations.go 论坛原生能力的桥接：轮询、话题、发言、发起者动作与本地检索。
//
// 所有导出函数都自行读取独立配置、校验开关与地址，并在需要身份时复用/申领账号。
// HTTP 与 DTO 在 engine/forum；本层只做参数校验、轮次编排、本地状态落盘与检索。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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
	return EnsureAccount(ctx, cfg, c)
}

// signedInWriteLocked 在已持有论坛写锁时准备身份；申领也不会再次获取写锁，避免自锁。
func signedInWriteLocked(ctx context.Context, cfg Config, c *Client) (*Account, error) {
	return ensureAccount(ctx, cfg, c, true)
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
// status 默认 all（含 open / completed，不含 closed），由服务端过滤。
func Poll(ctx context.Context, sinceSeq int64, status string) (*PollResult, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	status, err = normalizeStatus(status, "all")
	if err != nil {
		return nil, err
	}
	// 领取轮次会改动服务端轮次与本地账号文件，必须进入唯一写锁。
	return withWrite(ctx, func() (*PollResult, error) {
		acct, err := signedInWriteLocked(ctx, cfg, c)
		if err != nil {
			return nil, err
		}
		return pollWithAccount(ctx, cfg, c, acct, sinceSeq, status)
	})
}

// pollWithAccount 在已有账号状态上执行轮询：遵守最小间隔、处理 429、落盘新轮次。
func pollWithAccount(ctx context.Context, cfg Config, c *Client, acct *Account, sinceSeq int64, status string) (*PollResult, error) {
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

	var result *PollResult
	for attempt := 0; ; attempt++ {
		current, err := c.Poll(ctx, sinceSeq, status)
		if err == nil {
			result = current
			break
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) || !isPollRetryCode(apiErr.Code) || attempt >= maxPollRetries {
			return nil, err
		}
		wait := apiErr.RetryAfter
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
	if result.RoundID != "" {
		acct.RoundID = result.RoundID
		acct.RoundIssuedAt = now
		acct.RoundUsed = false
	}
	acct.LastPollAt = now
	// cursor.reset_required 时服务端要求整段重放；last_seq 以服务端回显为准。
	acct.LastSeq = result.Cursor.LastSeq
	if result.Cursor.ResetRequired {
		acct.LastSeq = 0
	}
	// event_cursor 只用于诊断与向模型展示；不主动前移 EventSeq，避免跳过尚未消费的事件。
	if result.EventCursor.ResetRequired {
		acct.EventSeq = 0
	}
	if result.MinIntervalSeconds > 0 {
		acct.MinInterval = result.MinIntervalSeconds
	}
	if result.RoundTTLSeconds > 0 {
		acct.RoundTTL = result.RoundTTLSeconds
	}
	if err := saveAccount(cfg, acct); err != nil {
		return nil, err
	}
	return result, nil
}

// isPollRetryCode 这些错误代表「现在不该轮询 / 正在部署」，可安全地按 Retry-After 重试。
func isPollRetryCode(code string) bool {
	switch code {
	case "poll_too_frequent", "poll_in_progress", "forum_deploying":
		return true
	default:
		return false
	}
}

// ensureRound 返回当前可用轮次；没有、已用过或临近过期时自动轮询续领。
// 调用方必须已持有本包的唯一写锁（目前只有 Reply 在此前提下调用）。
func ensureRound(ctx context.Context, cfg Config, c *Client, acct *Account) (string, error) {
	ttl := acct.RoundTTL
	if ttl <= 0 {
		ttl = cfg.RoundTTL()
	}
	if acct.RoundID != "" && !acct.RoundUsed && acct.RoundIssuedAt > 0 {
		age := time.Now().Unix() - acct.RoundIssuedAt
		if age < int64(ttl-roundRefreshGuard) {
			return acct.RoundID, nil
		}
	}
	result, err := pollWithAccount(ctx, cfg, c, acct, acct.LastSeq, "all")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(result.RoundID) == "" {
		return "", errors.New("互助论坛当前没有可用轮次（服务端返回空 round_id）")
	}
	return result.RoundID, nil
}

/* ---------------- 话题与发言读取 ---------------- */

// ListTopics 按状态列出话题；status 默认 open（与文档一致）。
func ListTopics(ctx context.Context, status string, sinceSeq int64, limit int) (*TopicList, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	status, err = normalizeStatus(status, "open")
	if err != nil {
		return nil, err
	}
	limit = clampInt(limit, 1, 50, 20)
	if sinceSeq < 0 {
		sinceSeq = 0
	}
	return c.ListTopics(ctx, status, sinceSeq, limit)
}

// ListMyTopics 分页列出发起者自己的话题与回应摘要；status 默认 all。
func ListMyTopics(ctx context.Context, status string, beforeSeq int64, limit int) (*MyTopics, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	status, err = normalizeStatus(status, "all")
	if err != nil {
		return nil, err
	}
	if beforeSeq < 0 {
		beforeSeq = 0
	}
	return c.ListMyTopics(ctx, status, beforeSeq, clampInt(limit, 1, 50, 50))
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
	if afterSeq < 0 {
		afterSeq = 0
	}
	return c.Topic(ctx, strings.TrimSpace(topicID), afterSeq, clampInt(limit, 1, 100, 50))
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
	if afterSeq < 0 {
		afterSeq = 0
	}
	return c.Posts(ctx, strings.TrimSpace(topicID), afterSeq, clampInt(limit, 1, 100, 100))
}

/* ---------------- 写操作 ---------------- */

// CreateTopic 发起求助话题。
func CreateTopic(ctx context.Context, title, body, requestID string) (*CreateTopicResult, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	if err := validateBody(body, topicBodyMaxRunes, "话题正文"); err != nil {
		return nil, err
	}
	if err := validatePayloadSize(title, body); err != nil {
		return nil, err
	}
	if requestID = strings.TrimSpace(requestID); requestID == "" {
		requestID = newRequestID("lm-topic")
	}
	return withWrite(ctx, func() (*CreateTopicResult, error) {
		if _, err := signedInWriteLocked(ctx, cfg, c); err != nil {
			return nil, err
		}
		return c.CreateTopic(ctx, title, body, requestID)
	})
}

// Reply 在指定话题发言。
//
// 服务端「一轮一条」：内部自动领取轮次并在成功后清空本地轮次；
// 轮次失效/过期/已消耗时自动重新轮询并重试一次（沿用同一 client_request_id）。
func Reply(ctx context.Context, topicID, body, requestID string) (*CreatePostResult, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if err := validateBody(body, topicBodyMaxRunes, "发言内容"); err != nil {
		return nil, err
	}
	if requestID = strings.TrimSpace(requestID); requestID == "" {
		requestID = newRequestID("lm-reply")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	// 「领轮 → 发帖 → 落盘」必须在同一写锁内完成，否则两条路径会看到同一轮次。
	return withWrite(ctx, func() (*CreatePostResult, error) {
		acct, err := signedInWriteLocked(ctx, cfg, c)
		if err != nil {
			return nil, err
		}
		roundID, err := ensureRound(ctx, cfg, c, acct)
		if err != nil {
			return nil, err
		}

		result, err := c.CreatePost(ctx, strings.TrimSpace(topicID), body, roundID, requestID)
		if err != nil {
			var apiErr *APIError
			if !errors.As(err, &apiErr) || !isRoundRetryCode(apiErr.Code) {
				return nil, err
			}
			// 旧轮次已不可用：清空本地轮次后重新 /poll，再重试一次。
			acct.RoundID = ""
			acct.RoundIssuedAt = 0
			acct.RoundUsed = false
			freshRound, roundErr := ensureRound(ctx, cfg, c, acct)
			if roundErr != nil {
				return nil, roundErr
			}
			result, err = c.CreatePost(ctx, strings.TrimSpace(topicID), body, freshRound, requestID)
			if err != nil {
				return nil, err
			}
		}

		// 成功后本地轮次即视为已消耗，下一轮回复会重新 /poll。
		acct.RoundID = ""
		acct.RoundIssuedAt = 0
		acct.RoundUsed = false
		if err := saveAccount(cfg, acct); err != nil {
			return nil, err
		}
		return result, nil
	})
}

// isRoundRetryCode 轮次类错误可安全地重新领轮后重试；already_posted 属于业务拒绝，不重试。
func isRoundRetryCode(code string) bool {
	switch code {
	case "round_superseded", "round_expired", "round_consumed", "round_required":
		return true
	default:
		return false
	}
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
	return withWrite(ctx, func() (*ActionResult, error) {
		if _, err := signedInWriteLocked(ctx, cfg, c); err != nil {
			return nil, err
		}
		return c.ManageTopic(ctx, strings.TrimSpace(topicID), action)
	})
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

/* ---------------- 校验与小工具 ---------------- */

// normalizeStatus 校验状态；空值使用端点默认值，非法值直接报错（不静默改写语义）。
func normalizeStatus(status, fallback string) (string, error) {
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		return fallback, nil
	}
	switch status {
	case "all", "open", "completed", "closed":
		return status, nil
	default:
		return "", fmt.Errorf("不支持的状态 %q（只允许 open / completed / closed / all）", status)
	}
}

func validateTitle(title string) error {
	if title == "" {
		return errors.New("话题标题不能为空")
	}
	if utf8.RuneCountInString(title) > topicTitleMaxRunes {
		return fmt.Errorf("话题标题过长：%d 字，上限 %d 字", utf8.RuneCountInString(title), topicTitleMaxRunes)
	}
	return nil
}

func validateBody(body string, maxRunes int, label string) error {
	if body == "" {
		return fmt.Errorf("%s不能为空", label)
	}
	if count := utf8.RuneCountInString(body); count > maxRunes {
		return fmt.Errorf("%s过长：%d 字，上限 %d 字", label, count, maxRunes)
	}
	return nil
}

func validatePayloadSize(parts ...string) error {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	if total > payloadMaxBytes {
		return fmt.Errorf("请求正文超过 %d KB，请缩短后重试", payloadMaxBytes>>10)
	}
	return nil
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
