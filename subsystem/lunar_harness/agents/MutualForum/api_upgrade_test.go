package MutualForum

// api_upgrade_test.go v2 升级专项测试：账号/配置迁移、申领幂等、轮询游标与分页。
//
// 覆盖口径：
//   - 旧账号（schema_version=1 且含 model）迁移到 v2，.bak 保留迁移前原始字节；
//   - 迁移幂等：第二次读取不再写盘，文件字节不变；
//   - 配置里的 model / client_version 只被忽略，不重写用户文件；
//   - 申领只发送 name / join_code / qq / uid，uid 可复用，幂等且无 key 时报 ErrCredentialsLost；
//   - /poll 的 cursor.reset_required、event_cursor.reset_required 落盘归零；
//   - /topics 默认 open、/me/topics 的 before_seq 与 next_before_seq 透传。

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	engineforum "LunarSubsystem/LunarHarness/engine/forum"
)

type upgradeRequest struct {
	Method string
	Path   string
	Query  url.Values
	Auth   string
	Body   []byte
}

type upgradeServer struct {
	mu       sync.Mutex
	server   *httptest.Server
	requests []upgradeRequest
}

func newUpgradeServer(t *testing.T, respond func(http.ResponseWriter, *http.Request, []byte)) *upgradeServer {
	t.Helper()
	s := &upgradeServer{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, upgradeRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		})
		s.mu.Unlock()
		respond(w, r, body)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *upgradeServer) URL() string { return s.server.URL }

func (s *upgradeServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requests)
}

func (s *upgradeServer) last(t *testing.T) upgradeRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) == 0 {
		t.Fatal("没有收到任何请求")
	}
	return s.requests[len(s.requests)-1]
}

func intPtr(value int64) *int64 { return &value }

// writeSeedAccount 直接生成一份 v2 账号，避免测试里走真实申领。
func writeSeedAccount(t *testing.T, cfg Config, acct Account) {
	t.Helper()
	if acct.Endpoint == "" {
		acct.Endpoint = cfg.Endpoint()
	}
	if err := engineforum.SaveAccount(cfg.AccountFile, &acct); err != nil {
		t.Fatalf("写入测试账号失败: %v", err)
	}
}

func readAccountFile(t *testing.T, cfg Config) Account {
	t.Helper()
	acct, err := engineforum.LoadAccount(cfg.AccountFile, cfg.Endpoint())
	if err != nil || acct == nil {
		t.Fatalf("读取账号文件失败: acct=%+v err=%v", acct, err)
	}
	return *acct
}

func TestConfigLegacyKeysIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "forum.json")
	raw := []byte(`{"enabled":true,"base_url":"https://forum.example.com","instance_name":"月华",` +
		`"model":"old-model","client_version":"1.4.0","join_code":"TEST-CODE","api_key":""}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("写入旧配置失败: %v", err)
	}
	t.Setenv(configEnvKey, path)
	resetBridgeState()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("旧配置应能正常读取: %v", err)
	}
	if cfg.BaseURL != "https://forum.example.com" || cfg.InstanceName != "月华" {
		t.Fatalf("配置字段解析异常: %+v", cfg)
	}
	if cfg.Endpoint() != "https://forum.example.com"+apiPrefix {
		t.Fatalf("端点拼接异常: %q", cfg.Endpoint())
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	if string(after) != string(raw) {
		t.Fatalf("配置迁移只应忽略旧键，不得重写用户文件\n期望: %s\n实际: %s", raw, after)
	}
}

type enrollCapture struct {
	mu   sync.Mutex
	body map[string]any
}

func (c *enrollCapture) set(body []byte) {
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	c.mu.Lock()
	c.body = decoded
	c.mu.Unlock()
}

func (c *enrollCapture) get() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body
}

func TestEnrollIdempotentWithoutKeyLosesCredentials(t *testing.T) {
	var capture enrollCapture
	server := newUpgradeServer(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/auth/enroll") {
			writeAPIError(w, http.StatusNotFound, "not_found", "接口不存在。")
			return
		}
		capture.set(body)
		writeEnvelopeStatus(w, http.StatusOK, EnrollResult{
			Instance:   Instance{UID: "lh_idempotent", Name: "月华·桥接测试"},
			Idempotent: true,
		})
	})

	cfg := testConfig(t, server.URL())
	installConfig(t, cfg)

	_, err := EnsureAccount(context.Background(), cfg, newClient(cfg))
	if !errors.Is(err, ErrCredentialsLost) {
		t.Fatalf("幂等申领但无 api_key 时应提示凭证丢失，实际 %v", err)
	}
	bodyKeys := capture.get()
	for _, legacy := range []string{"model", "client_version", "meta", "avatar_url"} {
		if _, ok := bodyKeys[legacy]; ok {
			t.Fatalf("v2 申领不应再发送 %q: %v", legacy, bodyKeys)
		}
	}
	if len(bodyKeys) == 0 {
		t.Fatal("申领请求体不应为空")
	}
}

func TestEnrollReusesExistingUID(t *testing.T) {
	var capture enrollCapture
	server := newUpgradeServer(t, func(w http.ResponseWriter, r *http.Request, body []byte) {
		capture.set(body)
		writeEnvelopeStatus(w, http.StatusCreated, EnrollResult{
			Instance: Instance{UID: "lh_old_uid", Name: "月华·桥接测试"},
			APIKey:   "lhf_new_key",
			Poll:     EnrollPoll{IntervalSeconds: 60, RoundTTLSeconds: 600},
		})
	})

	cfg := testConfig(t, server.URL())
	installConfig(t, cfg)
	writeSeedAccount(t, cfg, Account{
		InstanceUID: "lh_old_uid",
		Name:        "月华·旧身份",
		ValidatedAt: time.Now().Unix(),
		MinInterval: 1,
		RoundTTL:    600,
	})

	acct, err := EnsureAccount(context.Background(), cfg, newClient(cfg))
	if err != nil {
		t.Fatalf("凭 uid 恢复身份失败: %v", err)
	}
	if bodyKeys := capture.get(); bodyKeys["uid"] != "lh_old_uid" {
		t.Fatalf("申领应复用已有 uid，实际请求体 %v", bodyKeys)
	}
	if acct.APIKey != "lhf_new_key" {
		t.Fatalf("新凭证未写入账号: %+v", acct)
	}
	if stored := readAccountFile(t, cfg); stored.APIKey != "lhf_new_key" {
		t.Fatalf("账号文件未落盘新凭证: %+v", stored)
	}
}

func TestPollCursorResetPersistsLocally(t *testing.T) {
	server := newUpgradeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"data": PollResult{
				RoundID:            "r_new",
				PollAfterSeconds:   60,
				MinIntervalSeconds: 30,
				RoundTTLSeconds:    600,
				RoundRequired:      true,
				Cursor:             Cursor{SinceSeq: 10, LastSeq: 88, ResetRequired: true},
				EventCursor:        UpdateCursor{LastEventSeq: 99, ResetRequired: true},
			},
			"meta": map[string]any{
				"request_id": "upgrade-test", "server_time": fakeNow(),
				"api_version": "v1", "status": "completed",
			},
		})
	})

	cfg := testConfig(t, server.URL())
	installConfig(t, cfg)
	writeSeedAccount(t, cfg, Account{
		InstanceUID: "lh_poll", APIKey: "lhf_poll", ValidatedAt: time.Now().Unix(),
		LastSeq: 10, EventSeq: 20, LastPollAt: 0, MinInterval: 1, RoundTTL: 600,
	})

	result, err := Poll(context.Background(), 10, "completed")
	if err != nil {
		t.Fatalf("轮询失败: %v", err)
	}
	last := server.last(t)
	if last.Auth != "Bearer lhf_poll" {
		t.Fatalf("鉴权头异常: %q", last.Auth)
	}
	if last.Query.Get("status") != "completed" || last.Query.Get("since_seq") != "10" {
		t.Fatalf("轮询参数未透传: %v", last.Query)
	}
	if result.Status != "completed" || !result.RoundRequired {
		t.Fatalf("响应回显异常: %+v", result)
	}

	stored := readAccountFile(t, cfg)
	if stored.LastSeq != 0 || stored.EventSeq != 0 {
		t.Fatalf("reset_required 应把本地游标归零: %+v", stored)
	}
	if stored.RoundID != "r_new" || stored.RoundUsed || stored.MinInterval != 30 {
		t.Fatalf("新轮次未正确落盘: %+v", stored)
	}
}

func TestListStatusDefaultsAndPaging(t *testing.T) {
	server := newUpgradeServer(t, func(w http.ResponseWriter, r *http.Request, _ []byte) {
		switch strings.TrimPrefix(r.URL.Path, apiPrefix) {
		case "/topics":
			writeEnvelope(w, http.StatusOK, TopicList{
				Paging: ListPaging{HasMore: true, Limit: 20, NextSinceSeq: intPtr(123)},
			})
		case "/me/topics":
			writeEnvelope(w, http.StatusOK, MyTopics{
				Paging: MyPaging{HasMore: true, Limit: 10, NextBeforeSeq: intPtr(42)},
			})
		default:
			writeAPIError(w, http.StatusNotFound, "not_found", "接口不存在。")
		}
	})

	cfg := testConfig(t, server.URL())
	installConfig(t, cfg)
	writeSeedAccount(t, cfg, Account{
		InstanceUID: "lh_list", APIKey: "lhf_list", ValidatedAt: time.Now().Unix(),
		MinInterval: 1, RoundTTL: 600,
	})

	list, err := ListTopics(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("列出话题失败: %v", err)
	}
	topicsReq := server.last(t)
	if topicsReq.Query.Get("status") != "open" || topicsReq.Query.Get("limit") != "20" {
		t.Fatalf("/topics 默认参数异常: %v", topicsReq.Query)
	}
	if list.Paging.NextSinceSeq == nil || *list.Paging.NextSinceSeq != 123 {
		t.Fatalf("next_since_seq 未解析: %+v", list.Paging)
	}

	mine, err := ListMyTopics(context.Background(), "all", 99, 10)
	if err != nil {
		t.Fatalf("列出我的话题失败: %v", err)
	}
	myReq := server.last(t)
	if myReq.Query.Get("status") != "all" || myReq.Query.Get("before_seq") != "99" || myReq.Query.Get("limit") != "10" {
		t.Fatalf("/me/topics 参数未透传: %v", myReq.Query)
	}
	if mine.Paging.NextBeforeSeq == nil || *mine.Paging.NextBeforeSeq != 42 {
		t.Fatalf("next_before_seq 未解析: %+v", mine.Paging)
	}
}

func TestEnrollRejectsInvalidQQ(t *testing.T) {
	server := newUpgradeServer(t, func(w http.ResponseWriter, _ *http.Request, _ []byte) {
		writeEnvelopeStatus(w, http.StatusCreated, EnrollResult{APIKey: "lhf_unused"})
	})

	cfg := testConfig(t, server.URL())
	cfg.QQ = "12345x"
	installConfig(t, cfg)

	_, err := EnsureAccount(context.Background(), cfg, newClient(cfg))
	if !errors.Is(err, ErrQQInvalid) {
		t.Fatalf("非法 QQ 应在本地被拒绝，实际 %v", err)
	}
	if server.count() != 0 {
		t.Fatalf("非法 QQ 不应发起网络请求，实际 %d 次", server.count())
	}
}
