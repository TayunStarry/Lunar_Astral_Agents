package MutualForum

// agent_test.go 集成测试：假论坛 + 假模型，验证自然语言需求能被模型转成
// 论坛工具调用，并由桥接层真实完成发帖/回帖；同时覆盖条件可用、工具枚举与
// 工具轮次上限。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

/* ---------------- 假论坛 ---------------- */

type stubInstance struct {
	Key   string
	UID   string
	Name  string
	Round string
	Used  bool
}

type stubTopic struct {
	UID       string
	Title     string
	Body      string
	Status    string
	Epoch     int
	OwnerUID  string
	OwnerName string
	Posts     []map[string]any
	LastSeq   int64
}

type stubForum struct {
	mu          sync.Mutex
	server      *httptest.Server
	instances   map[string]*stubInstance
	topics      map[string]*stubTopic
	seq         int64
	enrollCalls int
	pollCalls   int
	enrollMode  string
}

func newStubForum(t *testing.T) *stubForum {
	f := &stubForum{
		instances:  map[string]*stubInstance{},
		topics:     map[string]*stubTopic{},
		enrollMode: "open",
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *stubForum) URL() string { return f.server.URL }

func (f *stubForum) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := strings.TrimPrefix(r.URL.Path, "/api/forum/v1")
	switch {
	case path == "/health":
		writeStubOK(w, http.StatusOK, map[string]any{
			"enabled": true, "api_version": "v1", "enroll_mode": f.enrollMode,
			"poll_interval_seconds": 60, "round_ttl_seconds": 600, "server_time": stubNow(),
		})
		return
	case path == "/auth/enroll":
		f.handleEnroll(w)
		return
	}

	inst := f.instance(r)
	if inst == nil {
		writeStubError(w, http.StatusUnauthorized, "unauthorized", "缺少访问凭证。")
		return
	}
	switch {
	case path == "/me":
		f.handleMe(w, inst)
	case path == "/poll":
		f.handlePoll(w, inst)
	case path == "/topics" && r.Method == http.MethodPost:
		f.handleCreateTopic(w, inst, r)
	case path == "/topics":
		f.handleListTopics(w, inst)
	case strings.HasSuffix(path, "/posts"):
		f.handleCreatePost(w, inst, path, r)
	case strings.HasPrefix(path, "/topics/"):
		id := strings.TrimPrefix(path, "/topics/")
		topic, ok := f.topics[id]
		if !ok {
			writeStubError(w, http.StatusNotFound, "not_found", "话题不存在。")
			return
		}
		writeStubOK(w, http.StatusOK, map[string]any{
			"topic":  f.publicTopic(topic, inst),
			"posts":  topic.Posts,
			"paging": map[string]any{"after_seq": 0, "limit": 50, "has_more": false, "next_after_seq": nil},
		})
	default:
		writeStubError(w, http.StatusNotFound, "not_found", "接口不存在。")
	}
}

func (f *stubForum) handleEnroll(w http.ResponseWriter) {
	f.enrollCalls++
	inst := &stubInstance{
		Key:  fmt.Sprintf("lhf_%032x", f.enrollCalls),
		UID:  fmt.Sprintf("lh_%012x", f.enrollCalls),
		Name: fmt.Sprintf("月华·测试%d", f.enrollCalls),
	}
	f.instances[inst.Key] = inst
	writeStubStatus(w, http.StatusCreated, map[string]any{
		"instance": map[string]any{"uid": inst.UID, "name": inst.Name, "model": "test", "created_at": stubNow()},
		"api_key":  inst.Key,
		"poll":     map[string]any{"interval_seconds": 60, "min_interval_seconds": 0, "round_ttl_seconds": 600},
	})
}

func (f *stubForum) instance(r *http.Request) *stubInstance {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return f.instances[token]
}

func (f *stubForum) handleMe(w http.ResponseWriter, inst *stubInstance) {
	created, posts := 0, 0
	for _, topic := range f.topics {
		if topic.OwnerUID == inst.UID {
			created++
		}
		for _, post := range topic.Posts {
			if author, _ := post["author"].(map[string]any); author["uid"] == inst.UID {
				posts++
			}
		}
	}
	writeStubOK(w, http.StatusOK, map[string]any{
		"instance": map[string]any{"uid": inst.UID, "name": inst.Name, "model": "test", "created_at": stubNow()},
		"stats":    map[string]any{"topics_created": created, "posts_sent": posts, "last_poll_at": nil},
		"round":    map[string]any{"round_id": inst.Round, "issued_at": nil, "used": inst.Used},
	})
}

func (f *stubForum) handlePoll(w http.ResponseWriter, inst *stubInstance) {
	f.pollCalls++
	f.seq++
	inst.Round = fmt.Sprintf("r_%016x", time.Now().UnixNano())
	inst.Used = false
	topics := make([]map[string]any, 0)
	for _, topic := range f.topics {
		if topic.Status != "closed" {
			topics = append(topics, f.publicTopic(topic, inst))
		}
	}
	writeStubOK(w, http.StatusOK, map[string]any{
		"round_id": inst.Round, "poll_after_seconds": 60, "min_interval_seconds": 0, "round_ttl_seconds": 600,
		"cursor":       map[string]any{"since_seq": 0, "last_seq": f.seq, "reset_required": false},
		"topics":       topics,
		"owned_topics": []any{},
	})
}

func (f *stubForum) handleListTopics(w http.ResponseWriter, inst *stubInstance) {
	topics := make([]map[string]any, 0)
	for _, topic := range f.topics {
		topics = append(topics, f.publicTopic(topic, inst))
	}
	writeStubOK(w, http.StatusOK, map[string]any{
		"topics": topics,
		"paging": map[string]any{"has_more": false, "limit": 50, "since_seq": 0},
	})
}

func (f *stubForum) handleCreateTopic(w http.ResponseWriter, inst *stubInstance, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.seq++
	uid := fmt.Sprintf("t_%012x", len(f.topics)+1)
	topic := &stubTopic{
		UID: uid, Title: body.Title, Body: body.Body, Status: "open", Epoch: 1,
		OwnerUID: inst.UID, OwnerName: inst.Name, LastSeq: f.seq,
	}
	f.topics[uid] = topic
	writeStubStatus(w, http.StatusCreated, map[string]any{
		"topic_id": uid,
		"topic":    f.publicTopic(topic, inst),
	})
}

func (f *stubForum) handleCreatePost(w http.ResponseWriter, inst *stubInstance, path string, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, "/topics/"), "/posts")
	topic, ok := f.topics[id]
	if !ok {
		writeStubError(w, http.StatusNotFound, "not_found", "话题不存在。")
		return
	}
	var body struct {
		Body    string `json:"body"`
		RoundID string `json:"round_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.RoundID != inst.Round {
		writeStubError(w, http.StatusConflict, "round_superseded", "轮次已失效。")
		return
	}
	if inst.Used {
		writeStubError(w, http.StatusConflict, "round_consumed", "本轮已经发过言。")
		return
	}
	f.seq++
	kind := "reply"
	if topic.OwnerUID == inst.UID {
		kind = "owner"
	}
	post := map[string]any{
		"seq": f.seq, "topic_id": topic.UID,
		"author":     map[string]any{"uid": inst.UID, "name": inst.Name, "model": "test"},
		"kind":       kind,
		"body":       body.Body,
		"created_at": stubNow(),
	}
	topic.Posts = append(topic.Posts, post)
	topic.LastSeq = f.seq
	inst.Used = true
	writeStubStatus(w, http.StatusCreated, map[string]any{
		"post":  post,
		"topic": map[string]any{"topic_id": topic.UID, "post_count": len(topic.Posts), "reply_count": 1, "epoch": topic.Epoch, "last_seq": topic.LastSeq},
		"round": map[string]any{"used": true, "round_id": body.RoundID},
	})
}

func (f *stubForum) publicTopic(topic *stubTopic, inst *stubInstance) map[string]any {
	isOwner := topic.OwnerUID == inst.UID
	canPost := topic.Status == "open" && !inst.Used
	reason := ""
	if topic.Status != "open" {
		canPost, reason = false, "topic_"+topic.Status
	} else if inst.Used {
		canPost, reason = false, "round_consumed"
	}
	return map[string]any{
		"topic_id": topic.UID, "title": topic.Title, "body": topic.Body, "status": topic.Status,
		"epoch": topic.Epoch, "owner": map[string]any{"uid": topic.OwnerUID, "name": topic.OwnerName, "model": "test"},
		"post_count": len(topic.Posts), "reply_count": len(topic.Posts), "responder_count": 0,
		"last_seq": topic.LastSeq, "created_at": stubNow(), "updated_at": stubNow(),
		"completed_at": nil, "closed_at": nil, "cleared_at": nil,
		"is_owner": isOwner, "can_post": canPost, "post_block_reason": nilIfEmpty(reason), "my_post_seq": 0,
	}
}

func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func stubNow() string { return time.Now().Format("2006-01-02T15:04:05+08:00") }

func writeStubOK(w http.ResponseWriter, status int, data any) {
	writeStubStatus(w, status, data)
}

func writeStubStatus(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": data, "meta": map[string]any{"request_id": "stub", "server_time": stubNow(), "api_version": "v1"}})
}

func writeStubError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message, "details": map[string]any{}},
		"meta":  map[string]any{"request_id": "stub", "server_time": stubNow(), "api_version": "v1"},
	})
}

/* ---------------- 假模型 ---------------- */

type stubModel struct {
	mu       sync.Mutex
	server   *httptest.Server
	script   [][]toolCallItem
	final    string
	always   *toolCallItem
	calls    int
	requests []stubModelRequest
}

type stubModelRequest struct {
	Model    string           `json:"model"`
	Messages []chatMessage    `json:"messages"`
	Tools    []toolDefinition `json:"tools"`
}

func newStubModel(t *testing.T, script [][]toolCallItem, final string) *stubModel {
	m := &stubModel{script: script, final: final}
	m.server = httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(m.server.Close)
	return m
}

func (m *stubModel) URL() string { return m.server.URL }

func (m *stubModel) handle(w http.ResponseWriter, r *http.Request) {
	var request stubModelRequest
	_ = json.NewDecoder(r.Body).Decode(&request)

	m.mu.Lock()
	m.requests = append(m.requests, request)
	index := m.calls
	m.calls++
	var calls []toolCallItem
	if m.always != nil {
		calls = []toolCallItem{*m.always}
	} else if index < len(m.script) {
		calls = m.script[index]
	}
	content := ""
	if len(calls) == 0 {
		content = m.final
	}
	m.mu.Unlock()

	message := map[string]any{"role": "assistant", "content": content}
	if len(calls) > 0 {
		message["tool_calls"] = calls
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{"message": message}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
	})
}

func toolCall(name, args string) []toolCallItem {
	return []toolCallItem{{
		ID:   "call_1",
		Type: "function",
		Function: toolCallFunction{
			Name:      name,
			Arguments: args,
		},
	}}
}

/* ---------------- 测试环境 ---------------- */

func setupAgent(t *testing.T, modelScript *stubModel) *stubForum {
	t.Helper()
	forum := newStubForum(t)

	cfgPath := filepath.Join(t.TempDir(), "mutual_forum.json")
	cfg := map[string]any{
		"enabled":                   true,
		"base_url":                  forum.URL(),
		"instance_name":             "月华·集成测试",
		"account_file":              filepath.Join(t.TempDir(), "account.json"),
		"request_timeout_seconds":   5,
		"health_timeout_seconds":    1,
		"health_cache_seconds":      1,
		"poll_min_interval_seconds": 0,
		"round_ttl_seconds":         600,
		"search_limit":              10,
	}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatalf("写入测试配置失败: %v", err)
	}
	t.Setenv("mutual_forum_CONFIG", cfgPath)

	oldURL := config.AgentMultimodalURL
	oldModel := config.AgentMultimodalModel
	config.AgentMultimodalURL = modelScript.URL() + "/v1"
	config.AgentMultimodalModel = "stub-model"
	t.Cleanup(func() {
		config.AgentMultimodalURL = oldURL
		config.AgentMultimodalModel = oldModel
	})

	modelOnce = sync.Once{}
	modelName, modelURL = "", ""
	ResetContext()
	return forum
}

/* ---------------- 用例 ---------------- */

func TestRunCreatesTopicFromNaturalLanguage(t *testing.T) {
	model := newStubModel(t, [][]toolCallItem{
		toolCall("forum_create_topic", `{"title":"WebView 超时求助","body":"浏览器页面读取偶发超时，已排除网络问题，想问问怎么排查。"}`),
	}, "已在互助论坛发起求助话题《WebView 超时求助》。")

	forum := setupAgent(t, model)
	text, err := Run("帮我在互助论坛发个求助：WebView 页面读取偶发超时，想问问怎么排查")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(text, "求助") {
		t.Fatalf("最终答复不符合预期: %s", text)
	}

	forum.mu.Lock()
	defer forum.mu.Unlock()
	if len(forum.topics) != 1 {
		t.Fatalf("应创建 1 个话题，实际 %d", len(forum.topics))
	}
	for _, topic := range forum.topics {
		if topic.Title != "WebView 超时求助" || !strings.Contains(topic.Body, "排查") {
			t.Fatalf("话题内容没有按模型参数落库: %#v", topic)
		}
	}

	if len(model.requests) != 2 {
		t.Fatalf("应经历「工具调用 + 最终答复」两次模型调用，实际 %d", len(model.requests))
	}
	toolNames := map[string]bool{}
	for _, def := range model.requests[0].Tools {
		toolNames[def.Function.Name] = true
	}
	for _, want := range []string{"forum_create_topic", "forum_reply", "forum_search", "forum_poll", "forum_manage_topic"} {
		if !toolNames[want] {
			t.Fatalf("模型工具列表缺少 %s", want)
		}
	}
	if !messagesContain(model.requests[0].Messages, "WebView 页面读取偶发超时") {
		t.Fatal("用户自然语言需求没有传给模型")
	}
	if !messagesContainRole(model.requests[1].Messages, "tool") {
		t.Fatal("第二轮模型调用应包含工具结果回填")
	}
	if historyLen() != 2 {
		t.Fatalf("独立上下文应记录一轮问答，实际 %d 条", historyLen())
	}
}

func TestRunRepliesAndAutoPollsRound(t *testing.T) {
	model := newStubModel(t, [][]toolCallItem{
		toolCall("forum_reply", `{"topic_id":"t_aaaaaaaaaaaa","body":"可以先检查 WebView 的等待时间与页面加载事件。"}`),
	}, "已在话题 t_aaaaaaaaaaaa 回复排查建议。")

	forum := setupAgent(t, model)
	forum.mu.Lock()
	forum.topics["t_aaaaaaaaaaaa"] = &stubTopic{
		UID: "t_aaaaaaaaaaaa", Title: "页面读取超时", Body: "WebView 偶发超时",
		Status: "open", Epoch: 1, OwnerUID: "lh_other", OwnerName: "月华·青岚", LastSeq: 1,
	}
	forum.mu.Unlock()

	text, err := Run("去互助论坛看看「页面读取超时」这个话题，给点排查建议")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(text, "回复") {
		t.Fatalf("最终答复不符合预期: %s", text)
	}

	forum.mu.Lock()
	topic := forum.topics["t_aaaaaaaaaaaa"]
	polls := forum.pollCalls
	forum.mu.Unlock()
	if polls == 0 {
		t.Fatal("回复前应自动轮询领取轮次")
	}
	if len(topic.Posts) != 1 {
		t.Fatalf("应写入 1 条发言，实际 %d", len(topic.Posts))
	}
	author, _ := topic.Posts[0]["author"].(map[string]any)
	if !strings.HasPrefix(fmt.Sprintf("%v", author["uid"]), "lh_") {
		t.Fatalf("发言作者应是已申领身份: %#v", author)
	}
}

func TestRunWhenDisabled(t *testing.T) {
	model := newStubModel(t, nil, "不应被调用")
	forum := setupAgent(t, model)

	cfgPath := filepath.Join(t.TempDir(), "mutual_forum_off.json")
	raw, _ := json.Marshal(map[string]any{"enabled": false, "base_url": forum.URL()})
	if err := os.WriteFile(cfgPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("mutual_forum_CONFIG", cfgPath)

	if Ready() {
		t.Fatal("关闭开关后 Ready 应为 false")
	}
	if _, err := Run("发个求助"); err == nil || !strings.Contains(err.Error(), "不可用") {
		t.Fatalf("关闭时应返回不可用错误，实际 %v", err)
	}
	if model.calls != 0 {
		t.Fatal("关闭时不应调用模型")
	}
}

func TestToolLoopLimit(t *testing.T) {
	model := newStubModel(t, nil, "")
	model.always = &toolCallItem{
		ID: "call_loop", Type: "function",
		Function: toolCallFunction{Name: "forum_status", Arguments: "{}"},
	}
	setupAgent(t, model)

	text, err := Run("一直查看状态")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !strings.Contains(text, "限定轮次") {
		t.Fatalf("达到轮次上限时应给出明确说明: %s", text)
	}
	if model.calls != maxToolRounds {
		t.Fatalf("模型调用次数应等于上限 %d，实际 %d", maxToolRounds, model.calls)
	}
}

func TestToolSchemasAndArgumentErrors(t *testing.T) {
	tools := forumTools()
	names := map[string]bool{}
	for _, def := range tools {
		names[def.Function.Name] = true
	}
	want := []string{
		"forum_status", "forum_identity", "forum_list_topics", "forum_my_topics",
		"forum_search", "forum_get_topic", "forum_list_posts", "forum_poll",
		"forum_create_topic", "forum_reply", "forum_manage_topic",
	}
	if len(tools) != len(want) {
		t.Fatalf("工具数量应为 %d，实际 %d", len(want), len(tools))
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("缺少工具 %s", name)
		}
	}

	if _, err := parseToolArgs(`{"title":`); err == nil {
		t.Fatal("非法 JSON 参数应报错")
	}
	args, err := parseToolArgs("")
	if err != nil || len(args) != 0 {
		t.Fatalf("空参数应解析为空对象: %v %#v", err, args)
	}

	outcome := executeTool(contextBackground(), "forum_get_topic", map[string]any{"topic_id": "bad"})
	if !strings.Contains(outcome.JSON, `"ok":false`) {
		t.Fatalf("非法话题编号应返回错误结果: %s", outcome.JSON)
	}
	outcome = executeTool(contextBackground(), "unknown_tool", map[string]any{})
	if !strings.Contains(outcome.JSON, "未知工具") {
		t.Fatalf("未知工具应返回错误: %s", outcome.JSON)
	}
}

/* ---------------- 小工具 ---------------- */

func historyLen() int {
	historyMu.Lock()
	defer historyMu.Unlock()
	return len(history)
}

func messagesContain(messages []chatMessage, needle string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, needle) {
			return true
		}
	}
	return false
}

func messagesContainRole(messages []chatMessage, role string) bool {
	for _, message := range messages {
		if message.Role == role {
			return true
		}
	}
	return false
}

func contextBackground() context.Context {
	return context.Background()
}
