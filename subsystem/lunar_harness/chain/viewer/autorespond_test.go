package viewer

// autorespond_test.go 生活链自动响应的行为测试：条件判定、三类来源、失败隔离与重试上限。

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func respondAt(suffix string, typ int) inboxItem {
	return inboxItem{
		Kind: "at", Category: notifyAt, Venue: "video", Type: typ,
		Oid: "4001", SourceID: "8001" + suffix, MessageID: "at" + suffix,
		User: "小A" + suffix, Message: "@月华 想听你展开讲讲" + suffix,
		Title: "我的视频", Time: time.Now().Unix(),
	}
}

func respondVideo(suffix string) inboxItem {
	return inboxItem{
		Kind: "reply", Category: notifyVideoComment, Venue: "video", Type: 1,
		Oid: "4002", SourceID: "8101" + suffix, MessageID: "video" + suffix,
		User: "观众" + suffix, Message: "背景音乐叫什么" + suffix,
		Title: "我的视频", Time: time.Now().Unix(),
	}
}

func respondDynamic(suffix string) inboxItem {
	return inboxItem{
		Kind: "reply", Category: notifyDynamic, Venue: "dynamic", Type: 17,
		Oid: "4003", SourceID: "8201" + suffix, MessageID: "dynamic" + suffix,
		User: "粉丝" + suffix, Message: "这条动态好有生活感" + suffix,
		Time: time.Now().Unix(),
	}
}

func TestAutoRespondTriggerConditions(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		setup  func(*respondTestEnv, *autoRespondDeps)
	}{
		{
			name:   "生活链未触发",
			reason: "生活链未触发",
			setup: func(_ *respondTestEnv, deps *autoRespondDeps) {
				deps.open = func() (autoRespondSession, error) {
					t.Fatal("生活链未触发时不应开启浏览器")
					return nil, nil
				}
			},
		},
		{
			name:   "页面开启失败",
			reason: "Bilibili 页面开启失败",
			setup: func(_ *respondTestEnv, deps *autoRespondDeps) {
				deps.open = func() (autoRespondSession, error) { return nil, os.ErrNotExist }
			},
		},
		{
			name:   "账号未登录",
			reason: "B站账号未登录",
			setup: func(env *respondTestEnv, _ *autoRespondDeps) {
				env.session.loginErr = os.ErrPermission
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			env, deps := newRespondTestEnv(t)
			env.session.items = []inboxItem{respondAt("1", 1)}
			test.setup(env, &deps)
			lifeTriggered := test.name != "生活链未触发"
			result, err := autoRespondOnBilibili(context.Background(), lifeTriggered, deps)
			if err != nil {
				t.Fatalf("条件不满足不应返回错误: %v", err)
			}
			if result.Triggered || result.Reason != test.reason {
				t.Fatalf("期望跳过并说明原因 %q，得到 triggered=%v reason=%q", test.reason, result.Triggered, result.Reason)
			}
			if env.session.fetched {
				t.Fatal("条件不满足时不应读取消息中心")
			}
			if len(env.replied) != 0 || len(env.activity) != 0 {
				t.Fatal("条件不满足时不应发布回应或更新活动状态")
			}
			if _, statErr := os.Stat(env.queuePath); !os.IsNotExist(statErr) {
				t.Fatal("条件不满足时不应写入待回应队列")
			}
		})
	}
}

func TestAutoRespondThreeSources(t *testing.T) {
	env, deps := newRespondTestEnv(t)
	env.session.items = []inboxItem{
		respondAt("1", 1),
		respondAt("2", 17), // @ 落在动态区，仍算 @ 来源
		respondVideo("1"),
		respondDynamic("1"),
	}
	result, err := autoRespondOnBilibili(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("自动响应失败: %v", err)
	}
	if !result.Triggered {
		t.Fatalf("三项条件满足时应执行，reason=%q", result.Reason)
	}
	if result.At.Found != 2 || result.At.Replied != 2 {
		t.Fatalf("@ 来源计数错误: %+v", result.At)
	}
	if result.Video.Found != 1 || result.Video.Replied != 1 {
		t.Fatalf("视频评论计数错误: %+v", result.Video)
	}
	if result.Dynamic.Found != 1 || result.Dynamic.Replied != 1 {
		t.Fatalf("动态评论计数错误: %+v", result.Dynamic)
	}
	if result.Replied() != 4 || result.Failed() != 0 {
		t.Fatalf("总数错误: replied=%d failed=%d", result.Replied(), result.Failed())
	}
	if len(env.replied) != 4 {
		t.Fatalf("应发布 4 条回应，实际 %d", len(env.replied))
	}
	queue, err := loadRespondQueueFile(env.queuePath)
	if err != nil || len(queue.Pending) != 0 {
		t.Fatalf("成功后队列应清空: pending=%d err=%v", len(queue.Pending), err)
	}
	if len(env.activity) != 1 || !strings.Contains(env.activity[0], "4 条") || !strings.Contains(env.activity[0], "@我 2 条") {
		t.Fatalf("活动状态文案不符合预期: %v", env.activity)
	}
}

func TestAutoRespondFailureDoesNotBlockOthers(t *testing.T) {
	env, deps := newRespondTestEnv(t)
	broken := respondVideo("2")
	env.session.items = []inboxItem{respondAt("3", 1), broken, respondDynamic("3")}
	env.failReply[broken.Message] = true

	result, err := autoRespondOnBilibili(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("单条失败不应让整批报错: %v", err)
	}
	if result.At.Replied != 1 || result.Dynamic.Replied != 1 {
		t.Fatalf("其余来源应继续成功: at=%+v dynamic=%+v", result.At, result.Dynamic)
	}
	if result.Video.Retrying != 1 || result.Video.Dropped != 0 || result.Failed() != 1 {
		t.Fatalf("失败条目应保留重试: %+v", result.Video)
	}
	queue, err := loadRespondQueueFile(env.queuePath)
	if err != nil {
		t.Fatalf("读取队列失败: %v", err)
	}
	if len(queue.Pending) != 1 {
		t.Fatalf("失败条目应留在队列，实际 %d 条", len(queue.Pending))
	}
	entry := queue.Pending[0]
	if entry.Source != sourceVideo || entry.Attempts != 1 || !strings.Contains(entry.LastError, "模拟发布失败") {
		t.Fatalf("队列条目状态错误: %+v", entry)
	}
	if env.seen[broken.Message] {
		t.Fatal("失败条目不应被标记为已处理，否则下一轮取不到")
	}
	if !env.seen[respondAt("3", 1).Message] || !env.seen[respondDynamic("3").Message] {
		t.Fatal("成功条目应标记已处理")
	}
}

func TestAutoRespondRetryCap(t *testing.T) {
	env, deps := newRespondTestEnv(t)
	item := respondVideo("4")
	env.session.items = []inboxItem{item}
	env.failGenerate[item.Message] = true
	ctx := context.Background()

	first, err := autoRespondOnBilibili(ctx, true, deps)
	if err != nil {
		t.Fatalf("首轮不应报错: %v", err)
	}
	if first.Video.Found != 1 || first.Video.Retrying != 1 {
		t.Fatalf("首轮失败应保留重试: %+v", first.Video)
	}

	// 消息中心不再返回该条，仍能从队列取到并继续重试。
	env.session.items = nil
	second, err := autoRespondOnBilibili(ctx, true, deps)
	if err != nil {
		t.Fatalf("第二轮不应报错: %v", err)
	}
	if second.Video.Retrying != 1 || second.Video.Dropped != 0 {
		t.Fatalf("第二轮应继续重试: %+v", second.Video)
	}
	queue, _ := loadRespondQueueFile(env.queuePath)
	if len(queue.Pending) != 1 || queue.Pending[0].Attempts != 2 {
		t.Fatalf("第二轮尝试次数应为 2: %+v", queue.Pending)
	}

	third, err := autoRespondOnBilibili(ctx, true, deps)
	if err != nil {
		t.Fatalf("第三轮不应报错: %v", err)
	}
	if third.Video.Dropped != 1 || third.Video.Retrying != 0 {
		t.Fatalf("达到上限应放弃并记录: %+v", third.Video)
	}
	queue, _ = loadRespondQueueFile(env.queuePath)
	if len(queue.Pending) != 0 {
		t.Fatalf("放弃后队列应为空，实际 %d 条", len(queue.Pending))
	}
	if len(queue.Dropped) != 1 || !strings.Contains(queue.Dropped[0].Reason, "模拟生成失败") {
		t.Fatalf("应记录放弃原因: %+v", queue.Dropped)
	}

	// 即使消息中心重新返回该条，也不应再入队重试。
	env.session.items = []inboxItem{item}
	fourth, err := autoRespondOnBilibili(ctx, true, deps)
	if err != nil {
		t.Fatalf("第四轮不应报错: %v", err)
	}
	if fourth.Video.Found != 0 || fourth.Video.Replied != 0 {
		t.Fatalf("放弃后不应再次处理: %+v", fourth.Video)
	}
}

func TestAutoRespondNothingToDoStillUpdatesActivity(t *testing.T) {
	env, deps := newRespondTestEnv(t)
	result, err := autoRespondOnBilibili(context.Background(), true, deps)
	if err != nil {
		t.Fatalf("无消息不应报错: %v", err)
	}
	if !result.Triggered || result.Replied() != 0 || result.Failed() != 0 {
		t.Fatalf("无消息时不应有回应计数: %+v", result)
	}
	if len(env.activity) != 1 || !strings.Contains(env.activity[0], "没有需要回应") {
		t.Fatalf("处理完成后应更新活动状态: %v", env.activity)
	}
}
