package MutualForum

// auto_reply_test.go 自动响应的编排测试：全部通过依赖注入的假实现完成，
// 不访问真实网络，也不读写真实账号文件。

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/activity"
	engineconfig "LunarSubsystem/LunarHarness/engine/config"
)

// memoryReplyStore 内存版重试队列，记录读写次数。
type memoryReplyStore struct {
	state   autoReplyState
	loadErr error
	saveErr error
	loads   int
	saves   int
}

func (m *memoryReplyStore) Load() (autoReplyState, error) {
	m.loads++
	if m.loadErr != nil {
		return autoReplyState{}, m.loadErr
	}
	return cloneReplyState(m.state), nil
}

func (m *memoryReplyStore) Save(state autoReplyState) error {
	m.saves++
	if m.saveErr != nil {
		return m.saveErr
	}
	m.state = cloneReplyState(state)
	return nil
}

func cloneReplyState(state autoReplyState) autoReplyState {
	out := state
	out.Items = append([]pendingReply(nil), state.Items...)
	return out
}

// fakeRespondDeps 记录调用次数的假依赖。
type fakeRespondDeps struct {
	pageErr       error
	newItems      []unreadReply
	pullErr       error
	pullCalls     int
	pullSince     []int64
	ackCalls      int
	store         *memoryReplyStore
	contextErr    map[string]error
	generateErr   map[string]error
	sendErr       map[int64]error
	generateCalls int
	sendCalls     int
	sentItems     []unreadReply
	activityText  []string
}

func newFakeRespondDeps(items ...unreadReply) *fakeRespondDeps {
	return &fakeRespondDeps{
		newItems:    items,
		store:       &memoryReplyStore{},
		contextErr:  map[string]error{},
		generateErr: map[string]error{},
		sendErr:     map[int64]error{},
	}
}

func (f *fakeRespondDeps) deps(maxRetries int) autoRespondDeps {
	return autoRespondDeps{
		ensurePage: func(context.Context) error { return f.pageErr },
		pullUnread: func(_ context.Context, since int64) (unreadBatch, error) {
			f.pullCalls++
			f.pullSince = append(f.pullSince, since)
			if f.pullErr != nil {
				return unreadBatch{}, f.pullErr
			}
			last := int64(0)
			for _, item := range f.newItems {
				if item.EventSeq > last {
					last = item.EventSeq
				}
			}
			return unreadBatch{Items: f.newItems, Cursor: UpdateCursor{LastEventSeq: last}}, nil
		},
		ackUnread: func(context.Context, unreadBatch) error {
			f.ackCalls++
			return nil
		},
		store:      f.store,
		maxRetries: maxRetries,
		loadContext: func(_ context.Context, item unreadReply) (replyContext, error) {
			if err := f.contextErr[item.PostID]; err != nil {
				return replyContext{}, err
			}
			return replyContext{EventType: item.Type, TopicID: item.TopicID, TopicBody: "原帖正文"}, nil
		},
		generate: func(_ context.Context, rc replyContext) (string, error) {
			f.generateCalls++
			if err := f.generateErr[rc.TopicID]; err != nil {
				return "", err
			}
			return fmt.Sprintf("针对 %s 的回复", rc.TopicID), nil
		},
		send: func(_ context.Context, item unreadReply, body string) error {
			f.sendCalls++
			if err := f.sendErr[item.EventSeq]; err != nil {
				return err
			}
			f.sentItems = append(f.sentItems, item)
			return nil
		},
		setActivity: func(text string) { f.activityText = append(f.activityText, text) },
	}
}

func TestExtractUnreadRepliesFiltersSelfAndNonReplyEvents(t *testing.T) {
	topicID, postID, commentID := "t_111111111111", "p_111111111111", "c_111111111111"
	events := []UpdateEvent{
		{EventSeq: 1, Type: "comment_created", IsSelf: false, TopicID: &topicID, PostID: &postID, CommentID: &commentID,
			Payload: map[string]any{"preview": "新评论"}},
		{EventSeq: 2, Type: "comment_created", IsSelf: true, TopicID: &topicID, PostID: &postID},
		{EventSeq: 3, Type: "like_received", IsSelf: false, TopicID: &topicID},
		{EventSeq: 4, Type: "mention", IsSelf: false, TopicID: &topicID, PostID: &postID},
		{EventSeq: 5, Type: "quote", IsSelf: false},
	}
	got := extractUnreadReplies(events)
	if len(got) != 2 {
		t.Fatalf("应只保留他人发出的回应类事件，实际 %d 条: %+v", len(got), got)
	}
	if got[0].EventSeq != 1 || got[0].CommentID != commentID || got[0].Preview != "新评论" {
		t.Fatalf("评论事件解析异常: %+v", got[0])
	}
	if got[1].EventSeq != 4 || got[1].Type != "mention" {
		t.Fatalf("提及事件解析异常: %+v", got[1])
	}
}

func TestMergePendingRepliesKeepsRetryState(t *testing.T) {
	existing := []pendingReply{{
		Key: "comment:c_111111111111", EventSeq: 3, Type: "comment_created",
		PostID: "p_111111111111", CommentID: "c_111111111111", Retries: 2, LastError: "旧错误",
	}}
	incoming := []unreadReply{{
		EventSeq: 3, Type: "comment_created", PostID: "p_111111111111", CommentID: "c_111111111111",
	}}
	merged := mergePendingReplies(existing, incoming)
	if len(merged) != 1 {
		t.Fatalf("同一目标不应重复入队，实际 %d 条", len(merged))
	}
	if merged[0].Retries != 2 || merged[0].LastError != "旧错误" {
		t.Fatalf("合并时应保留原重试状态: %+v", merged[0])
	}
}

func TestAutoRespondPageUnavailableMakesNoRequests(t *testing.T) {
	fake := newFakeRespondDeps(unreadReply{EventSeq: 1, TopicID: "t_111111111111"})
	fake.pageErr = errors.New("健康检查失败")
	summary, err := autoRespond(context.Background(), fake.deps(3))

	if !errors.Is(err, ErrForumPageUnavailable) {
		t.Fatalf("页面打不开应返回 ErrForumPageUnavailable，实际 %v", err)
	}
	if summary != (Summary{}) {
		t.Fatalf("页面打不开时不应返回有效 Summary: %+v", summary)
	}
	if fake.pullCalls != 0 || fake.store.loads != 0 || fake.store.saves != 0 || fake.generateCalls != 0 || fake.sendCalls != 0 {
		t.Fatalf("页面打不开时不应发起任何拉取或回复: %+v", fake)
	}
}

func TestAutoRespondNoUnreadMakesNoReplyRequests(t *testing.T) {
	fake := newFakeRespondDeps()
	summary, err := autoRespond(context.Background(), fake.deps(3))

	if err != nil {
		t.Fatalf("页面可用且无未读时不应返回错误: %v", err)
	}
	if !summary.PageOpen || summary.Pending != 0 || summary.Responded != 0 || summary.Failed != 0 || summary.Dropped != 0 {
		t.Fatalf("无未读时 Summary 不符: %+v", summary)
	}
	if fake.pullCalls != 1 || fake.generateCalls != 0 || fake.sendCalls != 0 {
		t.Fatalf("无未读时只应拉取一次，不生成也不提交: %+v", fake)
	}
	if len(fake.activityText) != 0 {
		t.Fatalf("无未读时不应更新活动状态: %v", fake.activityText)
	}
}

func TestAutoRespondProcessesEachUnreadInOrder(t *testing.T) {
	fake := newFakeRespondDeps(
		unreadReply{EventSeq: 1, Type: "comment_created", TopicID: "t_111111111111", PostID: "p_111111111111", CommentID: "c_111111111111"},
		unreadReply{EventSeq: 2, Type: "post_created", TopicID: "t_222222222222", PostID: "p_222222222222"},
		unreadReply{EventSeq: 3, Type: "mention", TopicID: "t_333333333333", PostID: "p_333333333333"},
	)
	summary, err := autoRespond(context.Background(), fake.deps(3))
	if err != nil {
		t.Fatalf("自动回应失败: %v", err)
	}
	if summary.Pending != 3 || summary.Responded != 3 || summary.Failed != 0 || summary.Dropped != 0 {
		t.Fatalf("计数不符: %+v", summary)
	}
	if len(fake.sentItems) != 3 {
		t.Fatalf("应逐条提交三条回复，实际 %d 条", len(fake.sentItems))
	}
	for index, item := range fake.sentItems {
		if item.EventSeq != int64(index+1) {
			t.Fatalf("回复顺序应与未读顺序一致，第 %d 条为 %+v", index, item)
		}
	}
	if len(fake.store.state.Items) != 0 {
		t.Fatalf("全部成功后队列应清空: %+v", fake.store.state.Items)
	}
	if len(fake.activityText) != 1 || !strings.Contains(fake.activityText[0], "3 条消息") {
		t.Fatalf("处理完应更新活动状态，实际 %v", fake.activityText)
	}
}

func TestAutoRespondSingleFailureDoesNotBlockOthers(t *testing.T) {
	fake := newFakeRespondDeps(
		unreadReply{EventSeq: 1, TopicID: "t_111111111111", PostID: "p_111111111111"},
		unreadReply{EventSeq: 2, TopicID: "t_222222222222", PostID: "p_222222222222"},
		unreadReply{EventSeq: 3, TopicID: "t_333333333333", PostID: "p_333333333333"},
	)
	fake.generateErr["t_222222222222"] = errors.New("模型调用失败")

	summary, err := autoRespond(context.Background(), fake.deps(3))
	if err != nil {
		t.Fatalf("单条失败不应整体报错: %v", err)
	}
	if summary.Pending != 3 || summary.Responded != 2 || summary.Failed != 1 || summary.Dropped != 0 {
		t.Fatalf("失败计数不符: %+v", summary)
	}
	if len(fake.sentItems) != 2 || fake.sentItems[0].EventSeq != 1 || fake.sentItems[1].EventSeq != 3 {
		t.Fatalf("失败条目不应阻塞其余条目: %+v", fake.sentItems)
	}
	if len(fake.store.state.Items) != 1 {
		t.Fatalf("失败条目应留在重试队列: %+v", fake.store.state.Items)
	}
	failed := fake.store.state.Items[0]
	if failed.EventSeq != 2 || failed.Retries != 1 || failed.LastError == "" {
		t.Fatalf("失败条目应记录重试次数与错误: %+v", failed)
	}
}

func TestAutoRespondRetriesFailedItemOnNextCall(t *testing.T) {
	fake := newFakeRespondDeps(
		unreadReply{EventSeq: 1, TopicID: "t_111111111111", PostID: "p_111111111111"},
		unreadReply{EventSeq: 2, TopicID: "t_222222222222", PostID: "p_222222222222"},
	)
	fake.sendErr[2] = errors.New("网络抖动")

	first, err := autoRespond(context.Background(), fake.deps(3))
	if err != nil {
		t.Fatalf("首轮不应整体报错: %v", err)
	}
	if first.Responded != 1 || first.Failed != 1 {
		t.Fatalf("首轮计数不符: %+v", first)
	}
	if fake.store.state.AckedSeq != 2 || len(fake.store.state.Items) != 1 {
		t.Fatalf("首轮后应保存游标与失败条目: %+v", fake.store.state)
	}

	// 下一轮：服务端没有新事件，失败条目仍应从本地队列取到并重试。
	fake.newItems = nil
	fake.sendErr = map[int64]error{}
	second, err := autoRespond(context.Background(), fake.deps(3))
	if err != nil {
		t.Fatalf("次轮不应整体报错: %v", err)
	}
	if second.Pending != 1 || second.Responded != 1 || second.Failed != 0 {
		t.Fatalf("次轮应重试并成功: %+v", second)
	}
	if len(fake.pullSince) != 2 || fake.pullSince[1] != 2 {
		t.Fatalf("次轮应从本地 AckedSeq 拉取，实际 %+v", fake.pullSince)
	}
	if len(fake.sentItems) != 2 || fake.sentItems[1].EventSeq != 2 {
		t.Fatalf("次轮应重新提交失败条目: %+v", fake.sentItems)
	}
	if len(fake.store.state.Items) != 0 {
		t.Fatalf("重试成功后队列应清空: %+v", fake.store.state.Items)
	}
}

func TestAutoRespondDropsItemAfterRetryLimit(t *testing.T) {
	fake := newFakeRespondDeps(
		unreadReply{EventSeq: 1, TopicID: "t_111111111111", PostID: "p_111111111111"},
		unreadReply{EventSeq: 2, TopicID: "t_222222222222", PostID: "p_222222222222"},
	)
	fake.sendErr[1] = errors.New("持续失败")

	first, err := autoRespond(context.Background(), fake.deps(2))
	if err != nil {
		t.Fatalf("首轮不应整体报错: %v", err)
	}
	if first.Responded != 1 || first.Failed != 1 || first.Dropped != 0 {
		t.Fatalf("首轮计数不符: %+v", first)
	}
	if len(fake.store.state.Items) != 1 || fake.store.state.Items[0].Retries != 1 {
		t.Fatalf("首轮后应保留一条待重试: %+v", fake.store.state.Items)
	}

	fake.newItems = nil
	second, err := autoRespond(context.Background(), fake.deps(2))
	if err != nil {
		t.Fatalf("次轮不应整体报错: %v", err)
	}
	if second.Responded != 0 || second.Failed != 1 || second.Dropped != 1 {
		t.Fatalf("达到上限后应计入 Dropped: %+v", second)
	}
	if len(fake.store.state.Items) != 0 {
		t.Fatalf("放弃后应从队列移除: %+v", fake.store.state.Items)
	}
	if fake.sendCalls != 3 {
		t.Fatalf("该条应尝试两次（另一条成功一次），实际发送 %d 次", fake.sendCalls)
	}

	fake.newItems = nil
	third, err := autoRespond(context.Background(), fake.deps(2))
	if err != nil {
		t.Fatalf("第三轮不应整体报错: %v", err)
	}
	if third.Pending != 0 || third.Responded != 0 || third.Failed != 0 {
		t.Fatalf("放弃后不应继续尝试: %+v", third)
	}
	if fake.sendCalls != 3 {
		t.Fatalf("放弃后不应再发送，实际发送 %d 次", fake.sendCalls)
	}
}

func TestAutoRespondUpdatesActivityField(t *testing.T) {
	oldDir := engineconfig.LocalDir
	engineconfig.LocalDir = t.TempDir()
	t.Cleanup(func() { engineconfig.LocalDir = oldDir })

	fake := newFakeRespondDeps(
		unreadReply{EventSeq: 1, TopicID: "t_111111111111", PostID: "p_111111111111"},
		unreadReply{EventSeq: 2, TopicID: "t_222222222222", PostID: "p_222222222222"},
	)
	deps := fake.deps(3)
	deps.setActivity = activity.Set

	summary, err := autoRespond(context.Background(), deps)
	if err != nil {
		t.Fatalf("自动回应失败: %v", err)
	}
	if summary.Responded != 2 {
		t.Fatalf("应回复两条，实际 %+v", summary)
	}
	if got := activity.Current(); got != "在互助论坛回复了 2 条消息" {
		t.Fatalf("活动状态字段未按预期更新: %q", got)
	}
	if line := activity.Line(); !strings.Contains(line, "在互助论坛回复了 2 条消息") {
		t.Fatalf("运行时消息未包含活动描述: %q", line)
	}
}

func TestFileReplyStateStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{AccountFile: filepath.Join(dir, "forum_account.json")}
	store := fileReplyStateStore{path: pendingStorePath(cfg)}
	if want := filepath.Join(dir, "forum_account.pending.json"); store.path != want {
		t.Fatalf("队列路径应由账号文件派生: got %s want %s", store.path, want)
	}

	empty, err := store.Load()
	if err != nil || len(empty.Items) != 0 || empty.AckedSeq != 0 {
		t.Fatalf("文件不存在时应返回空状态: %+v, %v", empty, err)
	}

	want := autoReplyState{
		AckedSeq: 42,
		Items: []pendingReply{{
			Key: "comment:c_111111111111", EventSeq: 42, Type: "comment_created",
			PostID: "p_111111111111", CommentID: "c_111111111111", Retries: 1, LastError: "网络抖动",
		}},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("保存队列失败: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("读取队列失败: %v", err)
	}
	if got.AckedSeq != 42 || len(got.Items) != 1 || got.Items[0].Retries != 1 || got.Items[0].LastError != "网络抖动" {
		t.Fatalf("队列回读不一致: %+v", got)
	}
}
