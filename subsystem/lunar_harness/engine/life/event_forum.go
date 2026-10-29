package life

// event_forum.go 逛论坛：生活体系内部调用互助论坛，不向对话者暴露入口。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/MutualForum"
)

type forumEvent struct{ params eventParams }

func newForumEvent() Event { return &forumEvent{params: paramsOf("forum")} }

func (e *forumEvent) ID() string       { return "forum" }
func (e *forumEvent) Weight() int      { return e.params.Weight }
func (e *forumEvent) DailyQuota() int  { return e.params.Quota }
func (e *forumEvent) NewRun() EventRun { return &forumRun{} }

type forumRunData struct {
	Instruction string `json:"instruction"`
}

type forumRun struct{ data forumRunData }

func (r *forumRun) Start(_ context.Context, env Env) error {
	// 工具触发的强制事件：直接采用对话者的自然语言要求引导论坛流程。
	if strings.TrimSpace(env.Instruction) != "" {
		r.data.Instruction = strings.TrimSpace(env.Instruction)
		return nil
	}
	r.data.Instruction = fmt.Sprintf(
		"以「%s」的状态去互助论坛逛一圈：先看看最近的新话题，挑一个感兴趣的读一读；"+
			"如果有能接话或能帮上忙的，就礼貌回一条；不要发广告、不要争论、不要暴露自己是程序。"+
			"刚刚我在：%s。",
		emptyAs(env.MoodLabel, "平静"), emptyAs(env.LastRecord, "没做什么"),
	)
	return nil
}

func (r *forumRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	if !MutualForum.Ready() {
		return EventStep{}, fmt.Errorf("互助论坛未就绪")
	}
	text, err := MutualForum.Run(r.data.Instruction)
	if err != nil {
		return EventStep{}, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "逛了一圈，没有特别想接话的"
	}
	return EventStep{
		Done:   true,
		Record: "逛了论坛：" + truncateAtSentence(text, 200),
		Detail: "论坛返回 " + fmt.Sprintf("%d 字", len([]rune(text))),
	}, nil
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
