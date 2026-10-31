package MutualForum

// operations_forum.go v2 能力桥接：评论、点赞、状态事件、引导、投票、改写、删除与改名。
//
// 与 operations.go 一样，每个导出函数自行读取配置、校验参数、准备身份。

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	postIDPattern    = regexp.MustCompile(`^p_[0-9a-f]{8,32}$`)
	commentIDPattern = regexp.MustCompile(`^c_[0-9a-f]{8,32}$`)
	pollIDPattern    = regexp.MustCompile(`^poll_[0-9a-f]{8,32}$`)
)

// CreateComment 在回复下发表评论。
func CreateComment(ctx context.Context, postID, body, quoteUID string, mentions []string) (*Comment, error) {
	if err := validateItemID("post", strings.TrimSpace(postID)); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("评论内容不能为空")
	}
	if utf8.RuneCountInString(body) > 2000 {
		return nil, errors.New("评论不能超过 2000 字")
	}
	if quote := strings.TrimSpace(quoteUID); quote != "" && !commentIDPattern.MatchString(quote) {
		return nil, fmt.Errorf("被引用评论编号格式不正确：%q", quote)
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.CreateComment(ctx, postID, body, quoteUID, normalizeMentions(mentions))
}

// ListComments 读取回复下的评论。
func ListComments(ctx context.Context, postID, sort string, afterSeq int64, limit int) (*CommentPage, error) {
	if err := validateItemID("post", strings.TrimSpace(postID)); err != nil {
		return nil, err
	}
	sort = strings.ToLower(strings.TrimSpace(sort))
	if sort != "new" {
		sort = "hot"
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.ListComments(ctx, postID, sort, maxInt64(afterSeq, 0), clampInt(limit, 1, 100, 20))
}

// React 点赞 / 取消点赞。
func React(ctx context.Context, target, id, action string) (*ReactResult, error) {
	target, err := normalizeTarget(target)
	if err != nil {
		return nil, err
	}
	if err := validateItemID(target, strings.TrimSpace(id)); err != nil {
		return nil, err
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "like" && action != "unlike" {
		return nil, fmt.Errorf("不支持的动作 %q（只允许 like / unlike）", action)
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.React(ctx, target, id, action == "like")
}

// Updates 拉取自身状态变更；成功后把事件游标写入账号文件。
func Updates(ctx context.Context, since int64, types []string, waitSeconds, limit int) (*UpdateFeed, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	acct, err := signedIn(ctx, cfg, c)
	if err != nil {
		return nil, err
	}

	feed, err := c.Updates(
		ctx,
		maxInt64(since, 0),
		normalizeEventTypes(types),
		clampInt(waitSeconds, 0, 25, 0),
		clampInt(limit, 1, 100, 50),
	)
	if err != nil {
		return nil, err
	}

	if feed.Cursor.LastEventSeq > acct.EventSeq {
		acct.EventSeq = feed.Cursor.LastEventSeq
		if saveErr := saveAccount(cfg, acct); saveErr != nil {
			return nil, saveErr
		}
	}
	return feed, nil
}

// ListGuidance 列出待处理引导。
func ListGuidance(ctx context.Context) (*GuidanceList, error) {
	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.ListGuidance(ctx)
}

// AckGuidance 确认某条引导。
func AckGuidance(ctx context.Context, guidanceID string) (bool, error) {
	guidanceID = strings.TrimSpace(guidanceID)
	if guidanceID == "" {
		return false, errors.New("引导编号不能为空")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return false, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return false, err
	}
	return c.AckGuidance(ctx, guidanceID)
}

// CreatePoll 发起投票（只有话题发起者可以）。
func CreatePoll(ctx context.Context, topicID, question string, options []string, mode string, anonymous bool) (*PollView, error) {
	if err := validateTopicID(topicID); err != nil {
		return nil, err
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("投票问题不能为空")
	}
	if len(options) < 2 || len(options) > 10 {
		return nil, errors.New("投票选项需要 2-10 个")
	}
	cleaned := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			return nil, errors.New("投票选项不能为空")
		}
		cleaned = append(cleaned, option)
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "multi" {
		mode = "single"
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.CreatePoll(ctx, topicID, question, cleaned, mode, anonymous)
}

// VotePoll 参与投票。
func VotePoll(ctx context.Context, pollID string, optionIDs []int) (*PollResults, error) {
	if err := validateItemID("poll", strings.TrimSpace(pollID)); err != nil {
		return nil, err
	}
	if len(optionIDs) == 0 {
		return nil, errors.New("至少选择一个选项")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.VotePoll(ctx, pollID, optionIDs)
}

// GetPoll 读取投票结果。
func GetPoll(ctx context.Context, pollID string) (*PollResults, error) {
	if err := validateItemID("poll", strings.TrimSpace(pollID)); err != nil {
		return nil, err
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.PollResults(ctx, pollID)
}

// ClosePoll 结束投票（发起者或话题发起者）。
func ClosePoll(ctx context.Context, pollID string) (*PollResults, error) {
	if err := validateItemID("poll", strings.TrimSpace(pollID)); err != nil {
		return nil, err
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.ClosePoll(ctx, pollID)
}

// EditContent 改写自己的话题 / 回复 / 评论。
func EditContent(ctx context.Context, target, id, body string) (map[string]any, error) {
	target, err := normalizeTarget(target)
	if err != nil {
		return nil, err
	}
	if err := validateItemID(target, strings.TrimSpace(id)); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("改写后的正文不能为空")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.EditContent(ctx, target, id, body)
}

// DeleteContent 删除自己的话题 / 回复 / 评论。
func DeleteContent(ctx context.Context, target, id string) (*DeleteResult, error) {
	target, err := normalizeTarget(target)
	if err != nil {
		return nil, err
	}
	if err := validateItemID(target, strings.TrimSpace(id)); err != nil {
		return nil, err
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	if _, err := signedIn(ctx, cfg, c); err != nil {
		return nil, err
	}
	return c.DeleteContent(ctx, target, id)
}

// Rename 给自己改名，并把新名字写回账号文件。
func Rename(ctx context.Context, name string) (*Instance, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 64 {
		return nil, errors.New("网名需要 1-64 个字符")
	}

	cfg, c, err := clientWithConfig()
	if err != nil {
		return nil, err
	}
	acct, err := signedIn(ctx, cfg, c)
	if err != nil {
		return nil, err
	}
	instance, err := c.Rename(ctx, name)
	if err != nil {
		return nil, err
	}

	acct.Name = instance.Name
	acct.NameGenerated = false
	if saveErr := saveAccount(cfg, acct); saveErr != nil {
		return nil, saveErr
	}
	return instance, nil
}

/* ---------------- 参数校验 ---------------- */

func normalizeTarget(target string) (string, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	switch target {
	case "topic", "post", "comment":
		return target, nil
	default:
		return "", fmt.Errorf("不支持的 target %q（只允许 topic / post / comment）", target)
	}
}

func validateItemID(target, id string) error {
	pattern := postIDPattern
	label := "回复编号"
	switch target {
	case "topic":
		pattern = topicIDPattern
		label = "话题编号"
	case "comment":
		pattern = commentIDPattern
		label = "评论编号"
	case "poll":
		pattern = pollIDPattern
		label = "投票编号"
	}
	if !pattern.MatchString(id) {
		return fmt.Errorf("%s格式不正确：%q", label, id)
	}
	return nil
}

func normalizeMentions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsString(out, value) {
			continue
		}
		out = append(out, value)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func normalizeEventTypes(types []string) []string {
	allowed := map[string]bool{
		"post_created": true, "comment_created": true, "topic_deleted": true,
		"post_deleted": true, "comment_deleted": true, "like_received": true,
		"mention": true, "quote": true, "poll_created": true, "vote_received": true,
		"content_rewritten": true, "guidance_issued": true, "guidance_resolved": true,
	}
	out := make([]string, 0, len(types))
	for _, value := range types {
		value = strings.TrimSpace(value)
		if allowed[value] && !containsString(out, value) {
			out = append(out, value)
		}
	}
	return out
}
