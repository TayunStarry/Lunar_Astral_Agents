package viewer

// autorespond_fake_test.go 自动响应的离线测试替身：不打开浏览器、不调模型、不写真实数据。

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// fakeRespondSession 只实现编排需要的会话能力，发布与生成都走注入的假实现。
type fakeRespondSession struct {
	loginErr error
	items    []inboxItem
	fetchErr error
	fetched  bool
	closed   bool
}

func (f *fakeRespondSession) ensureLogin() error { return f.loginErr }

func (f *fakeRespondSession) fetchInboxAllItems() ([]inboxItem, error) {
	f.fetched = true
	return f.items, f.fetchErr
}

func (f *fakeRespondSession) replyByAPI(inboxItem, string) error {
	return errors.New("测试不应走到真实发布")
}

func (f *fakeRespondSession) navigate(string, time.Duration) error { return nil }

func (f *fakeRespondSession) wait(time.Duration) {}

func (f *fakeRespondSession) uiReplyToComment(string, string, string) error {
	return errors.New("测试不应走到真实发布")
}

func (f *fakeRespondSession) close() { f.closed = true }

// respondTestEnv 记录假实现的副作用，便于断言。
type respondTestEnv struct {
	session      *fakeRespondSession
	queuePath    string
	replied      map[string]string
	seen         map[string]bool
	activity     []string
	failGenerate map[string]bool
	failReply    map[string]bool
}

// newRespondTestEnv 构造全部离线依赖；队列落在 t.TempDir()。
func newRespondTestEnv(t *testing.T) (*respondTestEnv, autoRespondDeps) {
	t.Helper()
	env := &respondTestEnv{
		session:      &fakeRespondSession{},
		queuePath:    filepath.Join(t.TempDir(), "autorespond.json"),
		replied:      map[string]string{},
		seen:         map[string]bool{},
		failGenerate: map[string]bool{},
		failReply:    map[string]bool{},
	}
	deps := defaultAutoRespondDeps()
	deps.open = func() (autoRespondSession, error) { return env.session, nil }
	deps.filter = func(items []inboxItem) []inboxItem {
		out := make([]inboxItem, 0, len(items))
		for _, item := range items {
			if !env.seen[item.Message] {
				out = append(out, item)
			}
		}
		return out
	}
	deps.generate = func(item inboxItem) (string, error) {
		if env.failGenerate[item.Message] {
			return "", errors.New("模拟生成失败")
		}
		return "回：" + item.Message, nil
	}
	deps.reply = func(_ autoRespondSession, item inboxItem, text string) error {
		if env.failReply[item.Message] {
			return errors.New("模拟发布失败")
		}
		env.replied[item.Message] = text
		return nil
	}
	deps.markSeen = func(item inboxItem) { env.seen[item.Message] = true }
	deps.loadQueue = func() (respondQueue, error) { return loadRespondQueueFile(env.queuePath) }
	deps.saveQueue = func(queue respondQueue) error { return saveRespondQueueFile(env.queuePath, queue) }
	deps.setActivity = func(text string) { env.activity = append(env.activity, text) }
	return env, deps
}
