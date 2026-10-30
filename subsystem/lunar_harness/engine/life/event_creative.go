package life

// event_creative.go 创作事件：生活体系触发一轮「月华.生活.创作」管线。
// 事件只负责准备触发信息并调用创作体系，规划日志与执行细节由创作体系维护。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/Creator"
)

type creativeEvent struct{ params eventParams }

func newCreativeEvent() Event { return &creativeEvent{params: paramsOf("creative")} }

func (e *creativeEvent) ID() string       { return "creative" }
func (e *creativeEvent) Weight() int      { return e.params.Weight }
func (e *creativeEvent) DailyQuota() int  { return e.params.Quota }
func (e *creativeEvent) NewRun() EventRun { return &creativeRun{} }

type creativeRunData struct {
	Instruction string `json:"instruction"`
}

type creativeRun struct{ data creativeRunData }

func (r *creativeRun) Start(_ context.Context, env Env) error {
	idea := strings.TrimSpace(env.LastUserText)
	if idea == "" {
		idea = "延续之前的创作思路，挑一件合适的作品或项目继续推进；也可以发起全新的创作。"
	}
	idea = truncateRunes(idea, 2000)
	r.data.Instruction = fmt.Sprintf(
		"以「%s」的状态进行本轮创作。刚才我在：%s。可以展开的创作方向：%s。"+
			"请先阅读创作规划日志，再把本轮要做的事编排成有序计划并执行。",
		emptyAs(env.MoodLabel, "平静"), emptyAs(env.LastRecord, "没做什么"), idea,
	)
	return nil
}

func (r *creativeRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	text, err := Creator.Run(ctx, r.data.Instruction)
	if err != nil {
		return EventStep{}, fmt.Errorf("创作失败: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "整理了一下创作思路，暂时没有落笔"
	}
	return EventStep{
		Done:   true,
		Record: "创作了：" + truncateAtSentence(text, 120),
		Detail: fmt.Sprintf("创作返回 %d 字", len([]rune(text))),
	}, nil
}

func (r *creativeRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *creativeRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}
