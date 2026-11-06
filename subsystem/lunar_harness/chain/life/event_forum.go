package life

// event_forum.go 逛论坛：生活体系内部调用互助论坛，不向对话者暴露入口。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"LunarSubsystem/LunarHarness/chain/forum"
)

type forumEvent struct{ params eventParams }

func newForumEvent() Event { return &forumEvent{params: paramsOf("forum")} }

func (e *forumEvent) ID() string       { return "forum" }
func (e *forumEvent) Weight() int      { return e.params.Weight }
func (e *forumEvent) DailyQuota() int  { return e.params.Quota }
func (e *forumEvent) NewRun() EventRun { return &forumRun{} }

type forumRunData struct {
	Instruction string `json:"instruction"`
	// AllowPost 本轮是否允许自主发新话题（由发帖冷却决定）
	AllowPost bool `json:"allow_post"`
}

type forumRun struct {
	data     forumRunData
	activity string
}

func (r *forumRun) Start(_ context.Context, env Env) error {
	// 工具触发的强制事件：直接采用对话者的自然语言要求引导论坛流程，不受发帖冷却限制。
	if instruction := strings.TrimSpace(env.Instruction); instruction != "" {
		r.data.Instruction = instruction
		r.data.AllowPost = true
		markForumPosted(env.Now)
		return nil
	}

	// 答复不限次；自主发新话题按冷却节流，避免每轮都刷帖。
	r.data.AllowPost = forumPostAllowed(env.Now)
	if r.data.AllowPost {
		markForumPosted(env.Now)
	}
	mood := emptyAs(env.MoodLabel, "平静")
	last := emptyAs(env.LastRecord, "没做什么")
	if r.data.AllowPost {
		r.data.Instruction = fmt.Sprintf(
			"以「%s」的状态去互助论坛逛一圈：先看看最近的新话题，挑一个感兴趣的读一读；"+
				"优先挑能接话或能帮上忙的话题回一条；确实没有合适的，或者自己这轮正好有想讨论的事，再发一个新话题。"+
				"不要发广告、不要争论、不要暴露自己是程序。刚刚我在：%s。",
			mood, last,
		)
	} else {
		r.data.Instruction = fmt.Sprintf(
			"以「%s」的状态去互助论坛逛一圈：先看看最近的新话题，挑一个感兴趣的读一读，"+
				"有能接话或能帮上忙的就礼貌回一条。本轮不要发新话题，只读和回复。"+
				"不要发广告、不要争论、不要暴露自己是程序。刚刚我在：%s。",
			mood, last,
		)
	}
	return nil
}

func (r *forumRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	if !forum.Ready() {
		return EventStep{}, skip("互助论坛未就绪")
	}
	text, err := forum.Run(r.data.Instruction)
	if err != nil {
		return EventStep{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "逛了一圈，没有特别想接话的"
	}
	// 操作完成立即更新状态字段；兜底写入点会用 ActivityText 保留这条更具体的文案。
	r.activity = "在互助论坛逛了逛：" + truncateAtSentence(text, activityTextLimit)
	setActivity(r.activity)
	return EventStep{
		Done:   true,
		Record: "逛了论坛：" + truncateAtSentence(text, 512),
		Detail: "论坛返回 " + fmt.Sprintf("%d 字", len([]rune(text))),
	}, nil
}

// ActivityText 返回分支内部已经写入的具体文案。
func (r *forumRun) ActivityText() string { return r.activity }

// forumPostAllowed 距离上次允许发帖是否已过冷却。
func forumPostAllowed(now time.Time) bool {
	mu.Lock()
	defer mu.Unlock()
	if state.ForumPostedAt <= 0 {
		return true
	}
	return now.Sub(time.Unix(state.ForumPostedAt, 0)) >= forumPostCooldown
}

// markForumPosted 记录本次允许发帖的时间（无论智能体最终发帖还是只回复，都占用一次窗口）。
func markForumPosted(now time.Time) {
	mu.Lock()
	defer mu.Unlock()
	state.ForumPostedAt = now.Unix()
	saveStateLocked()
}

func (r *forumRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *forumRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}
