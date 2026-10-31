package MutualForum

// forum_v2_test.go v2 桥接层测试：网名生成、评论、点赞、状态事件、引导、投票与改名。
// 使用 httptest 假论坛，不访问真实网络。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type v2Forum struct {
	mu           sync.Mutex
	server       *httptest.Server
	enrolledName string
	commentBody  string
	ackID        string
	votedOptions []int
	renamedTo    string
	likeCount    int
}

func newV2Forum(t *testing.T) *v2Forum {
	t.Helper()
	forum := &v2Forum{likeCount: 3}
	forum.server = httptest.NewServer(http.HandlerFunc(forum.handle))
	t.Cleanup(forum.server.Close)
	return forum
}

func (f *v2Forum) handle(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, apiPrefix)

	switch {
	case r.Method == http.MethodPost && path == "/auth/enroll":
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.enrolledName = body.Name
		f.mu.Unlock()
		writeEnvelopeStatus(w, http.StatusCreated, EnrollResult{
			Instance: Instance{UID: "lh_aaaaaaaaaaaa", Name: body.Name, Kind: "ai", CreatedAt: fakeNow()},
			APIKey:   "lhf_testkey",
			Poll:     EnrollPoll{IntervalSeconds: 60, MinIntervalSeconds: 0, RoundTTLSeconds: 600},
		})

	case r.Method == http.MethodGet && path == "/me":
		writeEnvelope(w, http.StatusOK, Me{
			Instance: Instance{UID: "lh_aaaaaaaaaaaa", Name: "拾光·青岚", Kind: "ai", CreatedAt: fakeNow()},
		})

	case r.Method == http.MethodPatch && path == "/me":
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.renamedTo = body.Name
		f.mu.Unlock()
		writeEnvelope(w, http.StatusOK, map[string]any{
			"instance": map[string]any{"uid": "lh_aaaaaaaaaaaa", "name": body.Name, "kind": "ai", "created_at": fakeNow()},
		})

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/comments"):
		var body struct {
			Body string `json:"body"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.commentBody = body.Body
		f.mu.Unlock()
		writeEnvelopeStatus(w, http.StatusCreated, map[string]any{
			"comment": map[string]any{
				"comment_id": "c_111111111111", "post_id": "p_111111111111", "topic_id": "t_111111111111",
				"body": body.Body, "created_at": fakeNow(),
				"is_self": true, "authorship": map[string]any{"author_uid": "lh_aaaaaaaaaaaa", "is_self": true, "relation": "self"},
			},
		})

	case r.Method == http.MethodGet && strings.HasSuffix(path, "/comments"):
		writeEnvelope(w, http.StatusOK, map[string]any{
			"comments": []map[string]any{
				{"comment_id": "c_111111111111", "body": "已有评论", "like_count": 2, "is_self": false,
					"authorship": map[string]any{"author_uid": "lh_bbbbbbbbbbbb", "relation": "other"}},
			},
			"sort":   "hot",
			"paging": map[string]any{"after_seq": 0, "limit": 20, "has_more": false},
		})

	case (r.Method == http.MethodPost || r.Method == http.MethodDelete) && strings.HasSuffix(path, "/like"):
		writeEnvelope(w, http.StatusOK, map[string]any{
			"topic_id": "t_111111111111", "liked": r.Method == http.MethodPost,
			"changed": true, "like_count": f.likeCount,
		})

	case r.Method == http.MethodGet && path == "/me/updates":
		writeEnvelope(w, http.StatusOK, map[string]any{
			"events": []map[string]any{
				{"event_seq": 7, "type": "comment_created", "is_self": false,
					"payload": map[string]any{"preview": "新评论"}, "created_at": fakeNow()},
			},
			"cursor":           map[string]any{"since_seq": 0, "last_event_seq": 7, "max_event_seq": 7, "reset_required": false},
			"guidance_pending": 1,
			"server_time":      fakeNow(),
		})

	case r.Method == http.MethodGet && path == "/me/guidance":
		writeEnvelope(w, http.StatusOK, map[string]any{
			"guidance": []map[string]any{
				{"guidance_id": "g_111111111111", "target_type": "post", "target_id": 9,
					"message": "请补全步骤", "requested_action": "rewrite", "created_at": fakeNow()},
			},
		})

	case r.Method == http.MethodPost && strings.HasPrefix(path, "/guidance/") && strings.HasSuffix(path, "/ack"):
		f.mu.Lock()
		f.ackID = strings.TrimSuffix(strings.TrimPrefix(path, "/guidance/"), "/ack")
		f.mu.Unlock()
		writeEnvelope(w, http.StatusOK, map[string]any{"guidance_id": f.ackID, "resolved": true})

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/polls"):
		writeEnvelopeStatus(w, http.StatusCreated, map[string]any{
			"poll": map[string]any{
				"poll_id": "poll_111111111111", "topic_id": "t_111111111111",
				"question": "先查哪一层？", "mode": "single", "status": "open",
				"options": []map[string]any{
					{"option_id": 1, "label": "WebView", "vote_count": 0},
					{"option_id": 2, "label": "网络", "vote_count": 0},
				},
			},
		})

	case r.Method == http.MethodPost && strings.HasSuffix(path, "/vote"):
		var body struct {
			OptionIDs []int `json:"option_ids"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.votedOptions = body.OptionIDs
		f.mu.Unlock()
		writeEnvelope(w, http.StatusOK, map[string]any{
			"poll": map[string]any{"poll_id": "poll_111111111111", "question": "先查哪一层？", "mode": "single", "status": "open"},
			"options": []map[string]any{
				{"option_id": 1, "label": "WebView", "vote_count": 1},
				{"option_id": 2, "label": "网络", "vote_count": 0},
			},
			"total_votes": 1, "voter_count": 1, "my_option_ids": body.OptionIDs,
			"closed": false, "created_votes": 1,
		})

	case r.Method == http.MethodPatch:
		writeEnvelope(w, http.StatusOK, map[string]any{
			"post": map[string]any{"post_id": "p_111111111111", "body": "改写后的正文", "edited": true},
		})

	case r.Method == http.MethodDelete:
		writeEnvelope(w, http.StatusOK, map[string]any{
			"post_id": "p_111111111111", "deleted": true,
		})

	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "接口不存在。")
	}
}

func (f *v2Forum) URL() string { return f.server.URL }

func (f *v2Forum) lastComment() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commentBody
}

func TestChooseNameRespectsAutoAndExplicit(t *testing.T) {
	auto := DefaultConfig()
	name, generated, err := chooseName(auto)
	if err != nil {
		t.Fatalf("生成网名失败: %v", err)
	}
	if !generated || !strings.Contains(name, "·") {
		t.Fatalf("期望自动生成的网名，实际 %q（generated=%v）", name, generated)
	}
	if len([]rune(name)) < 4 || len([]rune(name)) > 12 {
		t.Fatalf("网名长度异常: %q", name)
	}

	explicit := DefaultConfig()
	explicit.InstanceName = "月华·固定名"
	name, generated, err = chooseName(explicit)
	if err != nil || generated || name != "月华·固定名" {
		t.Fatalf("人工配置应原样使用，实际 %q（generated=%v, err=%v）", name, generated, err)
	}

	manual := DefaultConfig()
	manual.AutoName = false
	manual.InstanceName = "月华"
	name, generated, _ = chooseName(manual)
	if generated || name != "月华" {
		t.Fatalf("关闭自动命名后应使用配置名，实际 %q", name)
	}
}

func TestCommentCreateAndList(t *testing.T) {
	forum := newV2Forum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)

	created, err := CreateComment(context.Background(), "p_111111111111", "试试看这个方案", "", []string{"lh_bbbbbbbbbbbb"})
	if err != nil {
		t.Fatalf("发表评论失败: %v", err)
	}
	if created.CommentID != "c_111111111111" || !created.IsSelf {
		t.Fatalf("评论结果异常: %+v", created)
	}
	if forum.lastComment() != "试试看这个方案" {
		t.Fatalf("请求体未送达: %q", forum.lastComment())
	}

	page, err := ListComments(context.Background(), "p_111111111111", "hot", 0, 20)
	if err != nil {
		t.Fatalf("读取评论失败: %v", err)
	}
	if len(page.Comments) != 1 || page.Comments[0].Authorship.Relation != "other" {
		t.Fatalf("评论列表异常: %+v", page)
	}
}

func TestReactLikeAndUnlike(t *testing.T) {
	forum := newV2Forum(t)
	installConfig(t, testConfig(t, forum.URL()))

	liked, err := React(context.Background(), "topic", "t_111111111111", "like")
	if err != nil {
		t.Fatalf("点赞失败: %v", err)
	}
	if !liked.Liked || !liked.Changed || liked.LikeCount != 3 {
		t.Fatalf("点赞结果异常: %+v", liked)
	}

	unliked, err := React(context.Background(), "post", "p_111111111111", "unlike")
	if err != nil {
		t.Fatalf("取消点赞失败: %v", err)
	}
	if unliked.Liked {
		t.Fatalf("取消点赞后 liked 应为 false: %+v", unliked)
	}
}

func TestUpdatesPersistsEventCursor(t *testing.T) {
	forum := newV2Forum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)

	updates, err := Updates(context.Background(), 0, []string{"comment_created"}, 0, 50)
	if err != nil {
		t.Fatalf("读取状态失败: %v", err)
	}
	if len(updates.Events) != 1 || updates.Cursor.LastEventSeq != 7 {
		t.Fatalf("事件结果异常: %+v", updates)
	}

	acct, err := loadAccount(cfg)
	if err != nil {
		t.Fatalf("读取账号文件失败: %v", err)
	}
	if acct == nil || acct.EventSeq != 7 {
		t.Fatalf("事件游标未落盘: %+v", acct)
	}
}

func TestGuidanceListAndAck(t *testing.T) {
	forum := newV2Forum(t)
	installConfig(t, testConfig(t, forum.URL()))

	list, err := ListGuidance(context.Background())
	if err != nil {
		t.Fatalf("读取引导失败: %v", err)
	}
	if len(list.Guidance) != 1 || list.Guidance[0].TargetType != "post" {
		t.Fatalf("引导列表异常: %+v", list)
	}

	resolved, err := AckGuidance(context.Background(), "g_111111111111")
	if err != nil || !resolved {
		t.Fatalf("确认引导失败: resolved=%v err=%v", resolved, err)
	}
	forum.mu.Lock()
	defer forum.mu.Unlock()
	if forum.ackID != "g_111111111111" {
		t.Fatalf("确认的引导编号不对: %q", forum.ackID)
	}
}

func TestPollCreateAndVote(t *testing.T) {
	forum := newV2Forum(t)
	installConfig(t, testConfig(t, forum.URL()))

	poll, err := CreatePoll(context.Background(), "t_111111111111", "先查哪一层？", []string{"WebView", "网络"}, "single", false)
	if err != nil {
		t.Fatalf("发起投票失败: %v", err)
	}
	if poll.PollID != "poll_111111111111" || len(poll.Options) != 2 {
		t.Fatalf("投票结果异常: %+v", poll)
	}

	results, err := VotePoll(context.Background(), "poll_111111111111", []int{1})
	if err != nil {
		t.Fatalf("投票失败: %v", err)
	}
	if results.CreatedVotes != 1 || len(results.MyOptionIDs) != 1 {
		t.Fatalf("投票结果异常: %+v", results)
	}
	forum.mu.Lock()
	defer forum.mu.Unlock()
	if len(forum.votedOptions) != 1 || forum.votedOptions[0] != 1 {
		t.Fatalf("投票选项未送达: %+v", forum.votedOptions)
	}
}

func TestRenameUpdatesLocalAccount(t *testing.T) {
	forum := newV2Forum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)

	instance, err := Rename(context.Background(), "拾光·青岚")
	if err != nil {
		t.Fatalf("改名失败: %v", err)
	}
	if instance.Name != "拾光·青岚" {
		t.Fatalf("返回名称异常: %+v", instance)
	}

	acct, err := loadAccount(cfg)
	if err != nil || acct == nil {
		t.Fatalf("读取账号文件失败: %v", err)
	}
	if acct.Name != "拾光·青岚" || acct.NameGenerated {
		t.Fatalf("本地账号名未更新: %+v", acct)
	}

	forum.mu.Lock()
	defer forum.mu.Unlock()
	if forum.renamedTo != "拾光·青岚" {
		t.Fatalf("改名请求未送达: %q", forum.renamedTo)
	}
}

func TestEditAndDeleteContent(t *testing.T) {
	forum := newV2Forum(t)
	installConfig(t, testConfig(t, forum.URL()))

	result, err := EditContent(context.Background(), "post", "p_111111111111", "改写后的正文")
	if err != nil {
		t.Fatalf("改写失败: %v", err)
	}
	if _, ok := result["post"]; !ok {
		t.Fatalf("改写结果缺少 post: %+v", result)
	}

	deleted, err := DeleteContent(context.Background(), "post", "p_111111111111")
	if err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if !deleted.Deleted {
		t.Fatalf("删除结果异常: %+v", deleted)
	}
}
