package forum

// client_test.go 引擎层 httptest 契约测试：鉴权头、错误信封、分页、超时与瞬态重试。
//
// 只验证传输层与 DTO 解析；轮次重试、账号落盘等业务编排在 agents/MutualForum 的测试里。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeEnvelope 写一个统一成功信封。
func writeEnvelope(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": data,
		"meta": map[string]any{"request_id": "req_test", "api_version": "v2"},
	})
}

// writeAPIErrorEnvelope 写一个统一错误信封。
func writeAPIErrorEnvelope(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    code,
			"message": message,
			"details": map[string]any{"retry_after_seconds": 0},
		},
		"meta": map[string]any{"request_id": "req_err"},
	})
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := New(server.URL+"/api/forum/v1", 5*time.Second)
	client.SetAPIKey("lhf_test_key")
	return client, server
}

// TestHealthContractAndTransientRetry 覆盖 health 新增字段与 forum_deploying 的 GET 重试。
func TestHealthContractAndTransientRetry(t *testing.T) {
	var calls int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/forum/v1/health" {
			t.Errorf("健康检查路径错误: %s", r.URL.Path)
		}
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			writeAPIErrorEnvelope(w, http.StatusServiceUnavailable, "forum_deploying", "论坛正在部署。")
			return
		}
		writeEnvelope(w, http.StatusOK, Health{
			Enabled:                true,
			APIVersion:             "v2",
			EnrollMode:             "open",
			PollIntervalSeconds:    60,
			PollMinIntervalSeconds: 5,
			RoundTTLSeconds:        600,
			RoundRequired:          true,
			AvatarFallbackURL:      "media/img/avatar-default.svg",
		})
	})

	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("forum_deploying 应自动重试成功: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("期望重试 1 次（共 2 次请求），实际 %d", got)
	}
	if health.PollMinIntervalSeconds != 5 || !health.RoundRequired || health.AvatarFallbackURL == "" {
		t.Fatalf("health 新字段未解析: %+v", health)
	}
}

// TestTimeout 覆盖客户端超时。
func TestTimeout(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		writeEnvelope(w, http.StatusOK, Health{Enabled: true})
	})
	client = New(client.Endpoint, time.Second)
	client.SetAPIKey("lhf_test_key")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := client.Health(ctx)
	if err == nil {
		t.Fatal("超时应返回错误")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deadline") &&
		!strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("超时错误信息不明确: %v", err)
	}
}

// TestErrorEnvelopeBranches 覆盖 404/405 JSON 信封与 404 非 JSON 兜底。
func TestErrorEnvelopeBranches(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/forum/v1/topics/t_missing":
			writeAPIErrorEnvelope(w, http.StatusNotFound, "not_found", "话题不存在。")
		case "/api/forum/v1/topics/t_patch":
			w.Header().Set("Allow", "GET, POST")
			writeAPIErrorEnvelope(w, http.StatusMethodNotAllowed, "method_not_allowed", "不支持该请求方法。")
		case "/api/forum/v1/topics/t_html":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<html><body>404 Not Found</body></html>"))
		default:
			http.NotFound(w, r)
		}
	})

	var apiErr *APIError
	_, err := client.Topic(context.Background(), "t_missing", 0, 0)
	if !errors.As(err, &apiErr) || apiErr.Code != "not_found" || apiErr.Status != http.StatusNotFound {
		t.Fatalf("404 信封解析错误: %+v err=%v", apiErr, err)
	}

	_, err = client.EditContent(context.Background(), "topic", "t_patch", "x")
	if !errors.As(err, &apiErr) || apiErr.Code != "method_not_allowed" || apiErr.Allow == "" {
		t.Fatalf("405 未保留 Allow: %+v err=%v", apiErr, err)
	}
	if !strings.Contains(apiErr.Message, "PATCH/DELETE") {
		t.Fatalf("405 缺少前置服务器提示: %q", apiErr.Message)
	}

	_, err = client.Topic(context.Background(), "t_html", 0, 0)
	if !errors.As(err, &apiErr) || apiErr.Code != "not_found_html" {
		t.Fatalf("非 JSON 404 兜底错误: %+v err=%v", apiErr, err)
	}
}

// TestAuthHeaderAndQueryContracts 覆盖主要端点的鉴权头与查询参数。
func TestAuthHeaderAndQueryContracts(t *testing.T) {
	type seen struct {
		method string
		path   string
		query  url.Values
		auth   string
		accept string
		body   map[string]any
	}
	records := make([]seen, 0)
	record := func(w http.ResponseWriter, r *http.Request, data any) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		records = append(records, seen{
			method: r.Method, path: r.URL.Path, query: r.URL.Query(),
			auth: r.Header.Get("Authorization"), accept: r.Header.Get("Accept"), body: body,
		})
		writeEnvelope(w, http.StatusOK, data)
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/forum/v1/me":
			record(w, r, Me{
				Instance: Instance{UID: "lh_1", Name: "月华", QQMAsked: "12***56", AvatarURL: "media/avatar/lh_1.png"},
				Limits:   Limits{TestExempt: true},
			})
		case r.URL.Path == "/api/forum/v1/poll":
			if r.URL.Query().Get("status") != "open" || r.URL.Query().Get("since_seq") != "42" {
				t.Errorf("poll 查询参数错误: %s", r.URL.RawQuery)
			}
			result := PollResult{
				RoundID: "r_1", RoundRequired: true,
				Cursor:      Cursor{LastSeq: 7, ResetRequired: true},
				EventCursor: UpdateCursor{LastEventSeq: 3, ResetRequired: true},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "data": result,
				"meta": map[string]any{"status": "open", "owned_topics_order": "last_seq_desc"},
			})
		case r.URL.Path == "/api/forum/v1/topics":
			if r.URL.Query().Get("status") != "completed" || r.URL.Query().Get("since_seq") != "9" {
				t.Errorf("topics 查询参数错误: %s", r.URL.RawQuery)
			}
			record(w, r, TopicList{Topics: []Topic{{TopicID: "t_1"}}})
		case r.URL.Path == "/api/forum/v1/me/topics":
			if r.URL.Query().Get("before_seq") != "7" || r.URL.Query().Get("limit") != "20" {
				t.Errorf("me/topics 查询参数错误: %s", r.URL.RawQuery)
			}
			next := int64(3)
			record(w, r, MyTopics{Paging: MyPaging{BeforeSeq: 7, Limit: 20, HasMore: true, NextBeforeSeq: &next}})
		case r.URL.Path == "/api/forum/v1/posts/p_1/comments":
			if r.URL.Query().Get("sort") != "new" || r.URL.Query().Get("after_seq") != "3" {
				t.Errorf("comments 查询参数错误: %s", r.URL.RawQuery)
			}
			next := int64(0)
			record(w, r, CommentPage{Paging: CommentPaging{AfterSeq: 3, Limit: 10, NextAfterSeq: &next}})
		case r.URL.Path == "/api/forum/v1/me/guidance":
			if r.URL.Query().Get("limit") != "5" {
				t.Errorf("guidance limit 未透传: %s", r.URL.RawQuery)
			}
			record(w, r, GuidanceList{Guidances: []GuidanceItem{{GuidanceID: "g_1"}}})
		case r.URL.Path == "/api/forum/v1/topics/t_1" && r.Method == http.MethodPatch:
			record(w, r, map[string]any{"topic": Topic{TopicID: "t_1"}})
		case r.URL.Path == "/api/forum/v1/comments/c_1" && r.Method == http.MethodDelete:
			record(w, r, DeleteResult{CommentID: "c_1", Deleted: true})
		case r.URL.Path == "/api/forum/v1/polls/poll_1/close":
			record(w, r, PollResults{Closed: true})
		default:
			w.WriteHeader(http.StatusNotFound)
			writeAPIErrorEnvelope(w, http.StatusNotFound, "not_found", "路径不存在。")
		}
	})

	if _, err := client.Me(context.Background()); err != nil {
		t.Fatalf("me 失败: %v", err)
	}
	poll, err := client.Poll(context.Background(), 42, "open")
	if err != nil {
		t.Fatalf("poll 失败: %v", err)
	}
	if !poll.RoundRequired || !poll.Cursor.ResetRequired || !poll.EventCursor.ResetRequired {
		t.Fatalf("poll 新字段未解析: %+v", poll)
	}
	if poll.Status != "open" || poll.OwnedTopicsOrder != "last_seq_desc" {
		t.Fatalf("poll meta 未回填: status=%q order=%q", poll.Status, poll.OwnedTopicsOrder)
	}
	if _, err := client.ListTopics(context.Background(), "completed", 9, 15); err != nil {
		t.Fatalf("topics 失败: %v", err)
	}
	mine, err := client.ListMyTopics(context.Background(), "open", 7, 20)
	if err != nil {
		t.Fatalf("me/topics 失败: %v", err)
	}
	if mine.Paging.NextBeforeSeq == nil || *mine.Paging.NextBeforeSeq != 3 {
		t.Fatalf("next_before_seq 未解析: %+v", mine.Paging)
	}
	comments, err := client.ListComments(context.Background(), "p_1", "new", 3, 10)
	if err != nil {
		t.Fatalf("comments 失败: %v", err)
	}
	if comments.Paging.NextAfterSeq == nil || *comments.Paging.NextAfterSeq != 0 {
		t.Fatalf("next_after_seq=0 应保留为非空指针: %+v", comments.Paging)
	}
	if _, err := client.ListGuidance(context.Background(), 5); err != nil {
		t.Fatalf("guidance 失败: %v", err)
	}
	if _, err := client.EditContent(context.Background(), "topic", "t_1", "新正文"); err != nil {
		t.Fatalf("edit 失败: %v", err)
	}
	if _, err := client.DeleteContent(context.Background(), "comment", "c_1"); err != nil {
		t.Fatalf("delete 失败: %v", err)
	}
	if _, err := client.ClosePoll(context.Background(), "poll_1"); err != nil {
		t.Fatalf("close poll 失败: %v", err)
	}

	if len(records) == 0 {
		t.Fatal("没有记录到请求")
	}
	for _, rec := range records {
		if rec.path == "/api/forum/v1/me" || strings.HasPrefix(rec.path, "/api/forum/v1/poll") ||
			strings.HasPrefix(rec.path, "/api/forum/v1/topics") || strings.HasPrefix(rec.path, "/api/forum/v1/me/") ||
			strings.HasPrefix(rec.path, "/api/forum/v1/posts/") || strings.Contains(rec.path, "/comments") ||
			strings.Contains(rec.path, "/polls/") {
			if rec.auth != "Bearer lhf_test_key" {
				t.Fatalf("%s 鉴权头错误: %q", rec.path, rec.auth)
			}
			if rec.accept != "application/json" {
				t.Fatalf("%s Accept 头错误: %q", rec.path, rec.accept)
			}
		}
	}
}

// TestEnrollBodyContract 覆盖申领请求体只含 name/join_code/qq/uid，且不带鉴权。
func TestEnrollBodyContract(t *testing.T) {
	var raw map[string]any
	var path, method, auth string
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path, method, auth = r.URL.Path, r.Method, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&raw)
		writeEnvelope(w, http.StatusCreated, EnrollResult{
			Instance:   Instance{UID: "lh_9", Name: "月华"},
			APIKey:     "",
			Idempotent: true,
			Identity:   "uid:lh_9",
		})
	})

	result, err := client.Enroll(context.Background(), EnrollRequest{Name: "月华", JoinCode: "code", QQ: "12345678", UID: "lh_9"})
	if err != nil {
		t.Fatalf("enroll 失败: %v", err)
	}
	if path != "/api/forum/v1/auth/enroll" || method != http.MethodPost || auth != "" {
		t.Fatalf("enroll 请求契约错误: path=%s method=%s auth=%q", path, method, auth)
	}
	for _, forbidden := range []string{"model", "client_version", "meta", "avatar_url"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("enroll 不应发送字段 %q: %v", forbidden, raw)
		}
	}
	for _, required := range []string{"name", "join_code", "qq", "uid"} {
		if _, ok := raw[required]; !ok {
			t.Fatalf("enroll 缺少字段 %q: %v", required, raw)
		}
	}
	if !result.Idempotent || result.APIKey != "" || result.Instance.UID != "lh_9" {
		t.Fatalf("幂等响应解析错误: %+v", result)
	}
}

// TestPagingNullPointers 覆盖「空页为 null」的游标语义。
func TestPagingNullPointers(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"topics":[],"paging":{"has_more":false,"limit":20,"since_seq":0,"next_since_seq":null}}}`))
	})
	list, err := client.ListTopics(context.Background(), "open", 0, 20)
	if err != nil {
		t.Fatalf("topics 失败: %v", err)
	}
	if list.Paging.NextSinceSeq != nil {
		t.Fatalf("null next_since_seq 应为 nil: %+v", list.Paging)
	}
}
