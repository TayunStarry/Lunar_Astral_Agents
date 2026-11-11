package bilibili

// comment.go 视频/动态评论列表接口：页面上下文读取与公开高赞榜读取。

import (
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// rawComment 评论接口的响应条目。
type rawComment struct {
	Member struct {
		Uname string `json:"uname"`
	} `json:"member"`
	Content struct {
		Message string `json:"message"`
	} `json:"content"`
	Like int `json:"like"`
}

// CommentsOnPage 读取指定对象评论的指定页（typ：1=视频，17=纯文字动态）。
// byTime 为 true 时按时间排序，用于发布后回读自己的评论。
func CommentsOnPage(f Fetcher, typ int, oid int64, page, ps int, byTime bool) ([]Comment, error) {
	if typ <= 0 {
		typ = 1
	}
	if page < 1 {
		page = 1
	}
	if ps <= 0 || ps > commentPageSize {
		ps = commentPageSize
	}
	sort := 2
	if byTime {
		sort = 0
	}
	target := fmt.Sprintf("%s?type=%d&oid=%d&sort=%d&ps=%d&pn=%d", commentURL, typ, oid, sort, ps, page)
	text, err := f.FetchText("GET", target, "", "", commentPageTimeout)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []rawComment `json:"replies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("评论接口 code=%d", resp.Code)
	}
	out := make([]Comment, 0, len(resp.Data.Replies))
	for _, r := range resp.Data.Replies {
		if strings.TrimSpace(r.Content.Message) == "" {
			continue
		}
		out = append(out, Comment{User: r.Member.Uname, Content: r.Content.Message, Like: r.Like})
	}
	return out, nil
}

// NewCommentPager 构造逐页读取器（首页从 1 开始，单页条数上限 commentPageSize）。
func NewCommentPager(f Fetcher, typ int, oid int64, ps int, byTime bool) *CommentPager {
	if typ <= 0 {
		typ = 1
	}
	if ps <= 0 || ps > commentPageSize {
		ps = commentPageSize
	}
	return &CommentPager{f: f, typ: typ, oid: oid, page: 1, ps: ps, byTime: byTime}
}

// Next 读取下一页；返回 false 表示读取失败或已是空页。
func (p *CommentPager) Next() ([]Comment, bool) {
	if p == nil {
		return nil, false
	}
	items, err := CommentsOnPage(p.f, p.typ, p.oid, p.page, p.ps, p.byTime)
	if err != nil || len(items) == 0 {
		return nil, false
	}
	p.page++
	return items, true
}

// HighLikeComments 读取视频高赞评论（公开接口，按热榜翻页，最多 quickCommentMaxPages 页）。
// 失败只记录并返回已读到的部分，不影响调用方的主材料。
func HighLikeComments(aid int64, max int) []Comment {
	if aid <= 0 || max <= 0 {
		return nil
	}
	pages := (max + commentPageSize - 1) / commentPageSize
	if pages > quickCommentMaxPages {
		pages = quickCommentMaxPages
	}
	out := make([]Comment, 0, max)
	for page := 1; page <= pages; page++ {
		items := highLikeCommentPage(aid, page, commentPageSize)
		if len(items) == 0 {
			break
		}
		out = append(out, items...)
		if len(items) < commentPageSize {
			break
		}
	}
	return out
}

// highLikeCommentPage 读取高赞榜的指定页；接口不可用时返回 nil。
func highLikeCommentPage(aid int64, page, pageSize int) []Comment {
	target := fmt.Sprintf("%s?type=1&oid=%d&sort=2&ps=%d&pn=%d", commentURL, aid, pageSize, page)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Replies []rawComment `json:"replies"`
		} `json:"data"`
	}
	if err := getJSON(target, &resp); err != nil {
		logger.Warn("BilibiliViewer", "速览读取评论失败（继续）: %v", err)
		return nil
	}
	if resp.Code != 0 {
		logger.Warn("BilibiliViewer", "速览评论接口 code=%d（第 %d 页）", resp.Code, page)
		return nil
	}
	out := make([]Comment, 0, len(resp.Data.Replies))
	for _, item := range resp.Data.Replies {
		content := strings.TrimSpace(item.Content.Message)
		if content == "" {
			continue
		}
		out = append(out, Comment{
			User:    strings.TrimSpace(item.Member.Uname),
			Content: truncateRunes(content, commentMessageMaxRunes),
			Like:    item.Like,
		})
	}
	return out
}
