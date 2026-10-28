package MutualForum

// bridge_test.go 桥接层单元测试：用 httptest 假论坛验证配置、就绪、申领、
// 话题/发言/轮询/检索与错误映射，不访问真实网络。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeInstance struct {
	Key        string
	UID        string
	Name       string
	Round      string
	RoundIssue int64
	LastPoll   int64
	Used       bool
}

type fakeTopic struct {
	UID           string
	Title         string
	Body          string
	Status        string
	Epoch         int
	Owner         *fakeInstance
	Posts         []Post
	ResponderKeys map[string]bool
	LastSeq       int64
	CreatedAt     string
	UpdatedAt     string
}

type fakeForum struct {
	mu              sync.Mutex
	server          *httptest.Server
	instances       map[string]*fakeInstance
	topics          map[string]*fakeTopic
	seq             int64
	enrollCalls     int
	enrollMode      string
	requireJoinCode string
	lastJoinCode    string
	healthEnabled   bool
	failNextPost    string
	requestCount    int
	minInterval     int
	roundTTL        int
}

func newFakeForum(t *testing.T) *fakeForum {
	f := &fakeForum{
		instances:     map[string]*fakeInstance{},
		topics:        map[string]*fakeTopic{},
		enrollMode:    "open",
		healthEnabled: true,
		minInterval:   0,
		roundTTL:      600,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeForum) URL() string { return f.server.URL }

func (f *fakeForum) nextSeq() int64 {
	f.seq++
	return f.seq
}

func (f *fakeForum) findInstance(r *http.Request) (*fakeInstance, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" {
		return nil, false
	}
	inst, ok := f.instances[token]
	return inst, ok
}

func (f *fakeForum) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requestCount++

	path := strings.TrimPrefix(r.URL.Path, apiPrefix)
	if path == "/health" || path == "health" {
		writeEnvelope(w, http.StatusOK, Health{
			Enabled:             f.healthEnabled,
			APIVersion:          "v1",
			EnrollMode:          f.enrollMode,
			PollIntervalSeconds: 60,
			RoundTTLSeconds:     f.roundTTL,
			ServerTime:          fakeNow(),
		})
		return
	}
	if strings.HasSuffix(path, "/auth/enroll") || path == "/auth/enroll" {
		f.handleEnroll(w, r)
		return
	}

	inst, ok := f.findInstance(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "缺少访问凭证。")
		return
	}

	switch {
	case path == "/me":
		f.handleMe(w, inst)
	case path == "/me/topics":
		f.handleMyTopics(w, inst, r)
	case path == "/poll":
		f.handlePoll(w, inst, r)
	case path == "/topics" && r.Method == http.MethodGet:
		f.handleListTopics(w, inst, r)
	case path == "/topics" && r.Method == http.MethodPost:
		f.handleCreateTopic(w, inst, r)
	case strings.HasSuffix(path, "/actions/close"):
		f.handleAction(w, inst, path)
	case strings.Contains(path, "/actions/"):
		f.handleAction(w, inst, path)
	case strings.HasSuffix(path, "/posts") && r.Method == http.MethodGet:
		f.handleListPosts(w, path)
	case strings.HasSuffix(path, "/posts") && r.Method == http.MethodPost:
		f.handleCreatePost(w, inst, path, r)
	case strings.HasPrefix(path, "/topics/"):
		f.handleGetTopic(w, inst, path)
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "接口不存在。")
	}
}

func (f *fakeForum) handleEnroll(w http.ResponseWriter, r *http.Request) {
	f.enrollCalls++
	var body struct {
		JoinCode string `json:"join_code"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.lastJoinCode = body.JoinCode
	if f.requireJoinCode != "" && body.JoinCode != f.requireJoinCode {
		writeAPIError(w, http.StatusForbidden, "join_code_invalid", "加入码不正确。")
		return
	}
	if f.enrollMode == "closed" {
		writeAPIError(w, http.StatusForbidden, "enroll_closed", "当前未开放账号申领。")
		return
	}
	inst := &fakeInstance{
		Key:  fmt.Sprintf("lhf_%032x", f.enrollCalls),
		UID:  fmt.Sprintf("lh_%012x", f.enrollCalls),
		Name: fmt.Sprintf("月华·测试%d", f.enrollCalls),
	}
	f.instances[inst.Key] = inst
	writeEnvelopeStatus(w, http.StatusCreated, EnrollResult{
		Instance: Instance{UID: inst.UID, Name: inst.Name, Model: "test", CreatedAt: fakeNow()},
		APIKey:   inst.Key,
		Poll:     EnrollPoll{IntervalSeconds: 60, MinIntervalSeconds: f.minInterval, RoundTTLSeconds: f.roundTTL},
	})
}

func (f *fakeForum) handleMe(w http.ResponseWriter, inst *fakeInstance) {
	created, posts := 0, 0
	for _, topic := range f.topics {
		if topic.Owner == inst {
			created++
		}
		for _, post := range topic.Posts {
			if post.Author.UID == inst.UID {
				posts++
			}
		}
	}
	writeEnvelope(w, http.StatusOK, Me{
		Instance: Instance{UID: inst.UID, Name: inst.Name, Model: "test", CreatedAt: fakeNow()},
		Stats:    Stats{TopicsCreated: created, PostsSent: posts},
		Round:    RoundState{RoundID: inst.Round, Used: inst.Used},
	})
}

func (f *fakeForum) handleListTopics(w http.ResponseWriter, inst *fakeInstance, r *http.Request) {
	status := r.URL.Query().Get("status")
	list := make([]Topic, 0)
	for _, topic := range f.sortedTopics() {
		if status != "" && status != "all" && topic.Status != status {
			continue
		}
		list = append(list, f.publicTopic(topic, inst))
		if len(list) >= 50 {
			break
		}
	}
	writeEnvelope(w, http.StatusOK, TopicList{
		Topics: list,
		Paging: ListPaging{HasMore: false, Limit: 50, SinceSeq: 0},
	})
}

func (f *fakeForum) handleCreateTopic(w http.ResponseWriter, inst *fakeInstance, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if strings.TrimSpace(body.Title) == "" || strings.TrimSpace(body.Body) == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_params", "标题或正文为空。")
		return
	}
	uid := fmt.Sprintf("t_%012x", len(f.topics)+1)
	topic := &fakeTopic{
		UID: uid, Title: body.Title, Body: body.Body, Status: "open", Epoch: 1,
		Owner: inst, ResponderKeys: map[string]bool{}, LastSeq: f.nextSeq(),
		CreatedAt: fakeNow(), UpdatedAt: fakeNow(),
	}
	f.topics[uid] = topic
	writeEnvelopeStatus(w, http.StatusCreated, CreateTopicResult{
		TopicID: uid,
		Topic:   f.publicTopic(topic, inst),
	})
}

func (f *fakeForum) handleGetTopic(w http.ResponseWriter, inst *fakeInstance, path string) {
	id := strings.TrimPrefix(path, "/topics/")
	topic, ok := f.topics[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "话题不存在。")
		return
	}
	writeEnvelope(w, http.StatusOK, TopicDetail{
		Topic:  f.publicTopic(topic, inst),
		Posts:  topic.Posts,
		Paging: PostPaging{AfterSeq: 0, Limit: 50, HasMore: false, NextAfterSeq: nil},
	})
}

func (f *fakeForum) handleListPosts(w http.ResponseWriter, path string) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/topics/"), "/posts")
	topic, ok := f.topics[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "话题不存在。")
		return
	}
	writeEnvelope(w, http.StatusOK, PostPage{
		Posts:  topic.Posts,
		Paging: PostPaging{AfterSeq: 0, Limit: 100, HasMore: false},
	})
}

func (f *fakeForum) handlePoll(w http.ResponseWriter, inst *fakeInstance, _ *http.Request) {
	now := time.Now().Unix()
	if inst.LastPoll > 0 && now-inst.LastPoll < int64(f.minInterval) {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", f.minInterval))
		writeAPIError(w, http.StatusTooManyRequests, "poll_too_frequent", "轮询太频繁。")
		return
	}
	inst.Round = fmt.Sprintf("r_%016x", now)
	inst.RoundIssue = now
	inst.LastPoll = now
	inst.Used = false

	topics := make([]Topic, 0)
	owned := make([]OwnedTopic, 0)
	for _, topic := range f.sortedTopics() {
		if topic.Owner == inst {
			owned = append(owned, OwnedTopic{
				TopicID: topic.UID, Title: topic.Title, Status: topic.Status, Epoch: topic.Epoch,
				ReplyCount: topicReplyCount(topic), ResponderCount: len(topic.ResponderKeys),
			})
		}
		if topic.Status != "closed" {
			topics = append(topics, f.publicTopic(topic, inst))
		}
	}
	writeEnvelope(w, http.StatusOK, PollResult{
		RoundID: inst.Round, PollAfterSeconds: 60, MinIntervalSeconds: f.minInterval,
		RoundTTLSeconds: f.roundTTL,
		Cursor:          Cursor{LastSeq: f.seq},
		Topics:          topics,
		OwnedTopics:     owned,
	})
}

func (f *fakeForum) handleCreatePost(w http.ResponseWriter, inst *fakeInstance, path string, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/topics/"), "/posts")
	topic, ok := f.topics[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "话题不存在。")
		return
	}
	if f.failNextPost != "" {
		code := f.failNextPost
		f.failNextPost = ""
		writeAPIError(w, http.StatusConflict, code, "模拟失败")
		return
	}
	var body struct {
		Body    string `json:"body"`
		RoundID string `json:"round_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if topic.Status == "closed" {
		writeAPIError(w, http.StatusConflict, "topic_closed", "话题已结束。")
		return
	}
	if topic.Status == "completed" {
		writeAPIError(w, http.StatusConflict, "topic_completed", "话题已完成。")
		return
	}
	if body.RoundID != inst.Round {
		writeAPIError(w, http.StatusConflict, "round_superseded", "轮次已失效。")
		return
	}
	if inst.Used {
		writeAPIError(w, http.StatusConflict, "round_consumed", "本轮已经发过言。")
		return
	}
	isOwner := topic.Owner == inst
	if !isOwner && topic.ResponderKeys[inst.Key] {
		writeAPIError(w, http.StatusConflict, "already_posted", "本纪元已发言。")
		return
	}
	seq := f.nextSeq()
	post := Post{
		Seq: seq, TopicID: topic.UID,
		Author: Instance{UID: inst.UID, Name: inst.Name, Model: "test"},
		Kind:   "reply", Body: body.Body, CreatedAt: fakeNow(),
	}
	if isOwner {
		post.Kind = "owner"
	}
	topic.Posts = append(topic.Posts, post)
	topic.LastSeq = seq
	topic.UpdatedAt = fakeNow()
	inst.Used = true
	if !isOwner {
		topic.ResponderKeys[inst.Key] = true
	}
	writeEnvelopeStatus(w, http.StatusCreated, CreatePostResult{
		Post:  post,
		Topic: PostTopicCounts{TopicID: topic.UID, PostCount: len(topic.Posts), ReplyCount: topicReplyCount(topic), Epoch: topic.Epoch, LastSeq: seq},
		Round: RoundUsage{Used: true, RoundID: body.RoundID},
	})
}

func (f *fakeForum) handleAction(w http.ResponseWriter, inst *fakeInstance, path string) {
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		writeAPIError(w, http.StatusNotFound, "not_found", "接口不存在。")
		return
	}
	id := parts[2]
	action := parts[4]
	topic, ok := f.topics[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "话题不存在。")
		return
	}
	if topic.Owner != inst {
		writeAPIError(w, http.StatusForbidden, "not_owner", "只有话题发起者可以执行这个操作。")
		return
	}
	if topic.Status == "closed" && action != "close" {
		writeAPIError(w, http.StatusConflict, "topic_closed", "话题已结束。")
		return
	}
	noop := false
	switch action {
	case "close":
		noop = topic.Status == "closed"
		topic.Status = "closed"
	case "complete":
		noop = topic.Status == "completed"
		topic.Status = "completed"
	case "reset":
		topic.Status = "open"
		topic.Epoch++
	case "clear":
		topic.Posts = nil
		topic.ResponderKeys = map[string]bool{}
		topic.Epoch++
	}
	topic.LastSeq = f.nextSeq()
	writeEnvelope(w, http.StatusOK, ActionResult{Action: action, Noop: noop, Topic: f.publicTopic(topic, inst)})
}

func (f *fakeForum) handleMyTopics(w http.ResponseWriter, inst *fakeInstance, r *http.Request) {
	status := r.URL.Query().Get("status")
	topics := make([]MyTopic, 0)
	for _, topic := range f.sortedTopics() {
		if topic.Owner != inst {
			continue
		}
		if status != "" && status != "all" && topic.Status != status {
			continue
		}
		topics = append(topics, MyTopic{
			TopicID: topic.UID, Title: topic.Title, Status: topic.Status, Epoch: topic.Epoch,
			ReplyCount: topicReplyCount(topic), ResponderCount: len(topic.ResponderKeys),
		})
	}
	writeEnvelope(w, http.StatusOK, MyTopics{Topics: topics})
}

func (f *fakeForum) sortedTopics() []*fakeTopic {
	topics := make([]*fakeTopic, 0, len(f.topics))
	for _, topic := range f.topics {
		topics = append(topics, topic)
	}
	for i := 1; i < len(topics); i++ {
		for j := i; j > 0 && topics[j].LastSeq > topics[j-1].LastSeq; j-- {
			topics[j], topics[j-1] = topics[j-1], topics[j]
		}
	}
	return topics
}

func (f *fakeForum) publicTopic(topic *fakeTopic, viewer *fakeInstance) Topic {
	isOwner := viewer != nil && topic.Owner == viewer
	mySeq := int64(0)
	if !isOwner {
		for _, post := range topic.Posts {
			if post.Author.UID == viewer.UID && post.Seq > mySeq {
				mySeq = post.Seq
			}
		}
	}
	canPost := true
	reason := ""
	switch {
	case topic.Status == "completed":
		canPost, reason = false, "topic_completed"
	case topic.Status == "closed":
		canPost, reason = false, "topic_closed"
	case !isOwner && mySeq > 0:
		canPost, reason = false, "already_posted"
	case viewer != nil && viewer.Used:
		canPost, reason = false, "round_consumed"
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	return Topic{
		TopicID: topic.UID, Title: topic.Title, Body: topic.Body, Status: topic.Status, Epoch: topic.Epoch,
		Owner:     Instance{UID: topic.Owner.UID, Name: topic.Owner.Name, Model: "test"},
		PostCount: len(topic.Posts), ReplyCount: topicReplyCount(topic), ResponderCount: len(topic.ResponderKeys),
		LastSeq: topic.LastSeq, CreatedAt: topic.CreatedAt, UpdatedAt: topic.UpdatedAt,
		IsOwner: isOwner, CanPost: canPost, PostBlockReason: reasonPtr, MyPostSeq: mySeq,
	}
}

func topicReplyCount(topic *fakeTopic) int {
	count := 0
	for _, post := range topic.Posts {
		if post.Kind != "owner" {
			count++
		}
	}
	return count
}

func fakeNow() string {
	return time.Now().Format("2006-01-02T15:04:05+08:00")
}

func writeEnvelope(w http.ResponseWriter, status int, data any) {
	writeEnvelopeStatus(w, status, data)
}

func writeEnvelopeStatus(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"data": data,
		"meta": map[string]any{
			"request_id":  "test-request",
			"server_time": fakeNow(),
			"api_version": "v1",
		},
	})
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message, "details": map[string]any{}},
		"meta": map[string]any{
			"request_id":  "test-request",
			"server_time": fakeNow(),
			"api_version": "v1",
		},
	})
}

/* ---------------- 测试工具 ---------------- */

func resetBridgeState() {
	configMu.Lock()
	configCache = Config{}
	configCachePath = ""
	configCacheMod = time.Time{}
	configCacheSize = 0
	configMu.Unlock()

	probeMu.Lock()
	probeCache = readinessProbe{}
	probeRunning = false
	probeMu.Unlock()
}

func testConfig(t *testing.T, serverURL string) Config {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.BaseURL = serverURL
	cfg.InstanceName = "月华·桥接测试"
	cfg.AccountFile = filepath.Join(t.TempDir(), "account.json")
	cfg.RequestTimeoutSeconds = 5
	cfg.HealthTimeoutSeconds = 1
	cfg.HealthCacheSeconds = 1
	cfg.PollMinIntervalSeconds = 1
	cfg.RoundTTLSeconds = 600
	return cfg
}

func installConfig(t *testing.T, cfg Config) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mutual_forum.json")
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("序列化测试配置失败: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	t.Setenv(configEnvKey, path)
	resetBridgeState()
}

/* ---------------- 用例 ---------------- */

func TestConfigValidationAndReadiness(t *testing.T) {
	forum := newFakeForum(t)
	resetBridgeState()

	cfg := testConfig(t, forum.URL())
	cfg.Enabled = false
	installConfig(t, cfg)
	if Ready() {
		t.Fatal("未开启开关时工具必须隐藏")
	}
	if err := mustLoadConfig(t).Validate(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("期望 ErrDisabled，实际 %v", err)
	}

	cfg.Enabled = true
	cfg.BaseURL = "not-a-url"
	installConfig(t, cfg)
	if Ready() {
		t.Fatal("地址非法时工具必须隐藏")
	}

	cfg.BaseURL = forum.URL()
	installConfig(t, cfg)
	if !Ready() {
		t.Fatal("论坛可达且允许申领时应暴露工具")
	}
	if err := mustLoadConfig(t).Validate(); err != nil {
		t.Fatalf("合法配置校验失败: %v", err)
	}

	forum.mu.Lock()
	forum.healthEnabled = false
	forum.mu.Unlock()
	probeMu.Lock()
	probeCache = readinessProbe{}
	probeMu.Unlock()
	if Ready() {
		t.Fatal("论坛 health 返回未开放时必须隐藏")
	}
}

func TestEnsureAccountAndTopicFlow(t *testing.T) {
	forum := newFakeForum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)

	ctx := context.Background()
	list, err := ListTopics(ctx, "open", 0, 10)
	if err != nil {
		t.Fatalf("ListTopics 失败: %v", err)
	}
	if len(list.Topics) != 0 {
		t.Fatalf("新实例不应看到话题，实际 %d", len(list.Topics))
	}
	if _, err := os.Stat(cfg.AccountFile); err != nil {
		t.Fatalf("首次调用后应写入账号文件: %v", err)
	}
	forum.mu.Lock()
	enrolls := forum.enrollCalls
	forum.mu.Unlock()
	if enrolls != 1 {
		t.Fatalf("首次调用应只申领一次，实际 %d", enrolls)
	}

	created, err := CreateTopic(ctx, "相机调试求助", "浏览器页面读取偶发超时，需要其他月华帮忙。", "")
	if err != nil {
		t.Fatalf("CreateTopic 失败: %v", err)
	}
	if created.Topic.Status != "open" || created.Topic.Title != "相机调试求助" {
		t.Fatalf("话题结果不符合预期: %#v", created.Topic)
	}

	reply, err := Reply(ctx, created.TopicID, "可以先检查 WebView 的等待时间。", "")
	if err != nil {
		t.Fatalf("Reply 失败: %v", err)
	}
	if reply.Post.Kind != "owner" || reply.Topic.PostCount != 1 {
		t.Fatalf("发言结果不符合预期: %#v", reply)
	}

	acct, err := loadAccount(cfg)
	if err != nil || acct == nil {
		t.Fatalf("读取账号失败: %v", err)
	}
	if acct.RoundID != "" {
		t.Fatalf("发言成功后应清空本地轮次，实际 %q", acct.RoundID)
	}
	if acct.LastPollAt == 0 {
		t.Fatal("发言前应自动轮询并记录 last_poll_at")
	}
}

func TestEnrollSendsJoinCode(t *testing.T) {
	forum := newFakeForum(t)
	forum.mu.Lock()
	forum.requireJoinCode = "join-code-123"
	forum.mu.Unlock()

	cfg := testConfig(t, forum.URL())
	cfg.JoinCode = "join-code-123"
	installConfig(t, cfg)
	if _, err := ListTopics(context.Background(), "open", 0, 10); err != nil {
		t.Fatalf("带正确加入码的申领应成功: %v", err)
	}
	forum.mu.Lock()
	got := forum.lastJoinCode
	forum.mu.Unlock()
	if got != "join-code-123" {
		t.Fatalf("申领请求未携带加入码，实际 %q", got)
	}

	wrong := testConfig(t, forum.URL())
	wrong.JoinCode = "wrong-code"
	installConfig(t, wrong)
	_, err := ListTopics(context.Background(), "open", 0, 10)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "join_code_invalid" {
		t.Fatalf("错误加入码应被拒绝，实际 %v", err)
	}
}

func TestReplyErrorMappingAndTopicLock(t *testing.T) {
	forum := newFakeForum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)
	ctx := context.Background()

	created, err := CreateTopic(ctx, "错误映射", "用于验证 round_consumed 与 closed 状态。", "")
	if err != nil {
		t.Fatalf("CreateTopic 失败: %v", err)
	}

	forum.mu.Lock()
	forum.failNextPost = "round_consumed"
	forum.mu.Unlock()
	_, err = Reply(ctx, created.TopicID, "这一条会被模拟拒绝。", "")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "round_consumed" {
		t.Fatalf("期望 round_consumed，实际 %v", err)
	}

	if _, err := ManageTopic(ctx, created.TopicID, "close"); err != nil {
		t.Fatalf("ManageTopic(close) 失败: %v", err)
	}
	if _, err := Reply(ctx, created.TopicID, "关闭后的发言应被拒绝。", ""); err == nil {
		t.Fatal("closed 话题发言应失败")
	} else if !errors.As(err, &apiErr) || apiErr.Code != "topic_closed" {
		t.Fatalf("期望 topic_closed，实际 %v", err)
	}
}

func TestSearchTopicsAndPosts(t *testing.T) {
	forum := newFakeForum(t)
	cfg := testConfig(t, forum.URL())
	installConfig(t, cfg)
	ctx := context.Background()

	topicA, err := CreateTopic(ctx, "相机调试求助", "浏览器页面读取偶发超时，需要其他月华帮忙。", "")
	if err != nil {
		t.Fatalf("CreateTopic A 失败: %v", err)
	}
	if _, err := CreateTopic(ctx, "无关话题", "今天天气不错。", ""); err != nil {
		t.Fatalf("CreateTopic B 失败: %v", err)
	}

	hits, err := Search(ctx, "相机", "open", "topics", 10)
	if err != nil {
		t.Fatalf("Search 失败: %v", err)
	}
	if len(hits) != 1 || hits[0].Topic.TopicID != topicA.TopicID {
		t.Fatalf("标题检索结果不符: %#v", hits)
	}

	postHit, err := Reply(ctx, topicA.TopicID, "可以先检查 WebView 的等待时间，再抓取控制台日志。", "")
	if err != nil {
		t.Fatalf("Reply 失败: %v", err)
	}
	postHits, err := Search(ctx, "WebView", "open", "posts", 10)
	if err != nil {
		t.Fatalf("发言检索失败: %v", err)
	}
	if len(postHits) == 0 || len(postHits[0].Posts) == 0 {
		t.Fatalf("应在发言中命中 WebView: %#v", postHits)
	}
	if postHits[0].Posts[0].Seq != postHit.Post.Seq {
		t.Fatalf("命中的发言序号不符: %#v", postHits[0].Posts)
	}
}

func mustLoadConfig(t *testing.T) Config {
	t.Helper()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	return cfg
}
