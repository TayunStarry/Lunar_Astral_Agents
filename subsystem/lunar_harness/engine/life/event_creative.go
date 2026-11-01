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
	Instruction string   `json:"instruction"`
	Directives  []string `json:"directives,omitempty"`
}

type creativeRun struct{ data creativeRunData }

func (r *creativeRun) Start(_ context.Context, env Env) error {
	// 工具触发的强制指令优先于最新发言，避免被别的会话覆盖。
	requirement := strings.TrimSpace(env.Instruction)
	idea := requirement
	if idea == "" {
		idea = strings.TrimSpace(env.LastUserText)
	}
	if idea == "" {
		idea = "延续之前的创作思路，挑一件合适的作品或项目继续推进；也可以发起全新的创作。"
	}
	idea = truncateRunes(idea, 2000)
	focus := "可以展开的创作方向（来自月华与用户最近的交流）：" + idea + "。"
	if requirement != "" {
		focus = "本轮必须完成的要求（工具触发的强制指令）：" + idea + "。"
	}
	r.data.Instruction = fmt.Sprintf(
		"这是月华本人的当前状态（心情与生活记录都属于月华自己，不是用户的状态）："+
			"当前心情「%s」；刚才月华在：%s。%s"+
			"请先阅读创作规划日志，再把本轮要做的事编排成有序计划并执行。",
		emptyAs(env.MoodLabel, "平静"), emptyAs(env.LastRecord, "没做什么"), focus,
	)
	r.data.Directives = recentUserTexts()
	return nil
}

// recentUserTexts 汇总最近若干轮用户发言，供创作体系识别用户的明确要求。
func recentUserTexts() []string {
	mu.Lock()
	hook := dialogueRoundsHook
	mu.Unlock()
	if hook == nil {
		return nil
	}
	rounds := hook()
	out := make([]string, 0, len(rounds))
	for _, round := range rounds {
		text := strings.TrimSpace(round.UserText)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func (r *creativeRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	text, err := Creator.Run(ctx, r.data.Instruction, r.data.Directives)
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
