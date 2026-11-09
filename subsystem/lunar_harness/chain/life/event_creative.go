package life

// event_creative.go 创作事件：生活体系触发一轮「月华.生活.创作」管线。
// 创作方向直接读取心愿链路的愿望清单，不再维护单独的创作意图文件；
// 心情正负决定是延续上一轮思路，还是允许自行规划。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/chain/creation"
	"LunarSubsystem/LunarHarness/chain/wishchain"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

type creativeEvent struct{ params eventParams }

func newCreativeEvent() Event { return &creativeEvent{params: paramsOf("creative")} }

func (e *creativeEvent) ID() string       { return "creative" }
func (e *creativeEvent) Weight() int      { return e.params.Weight }
func (e *creativeEvent) DailyQuota() int  { return e.params.Quota }
func (e *creativeEvent) NewRun() EventRun { return &creativeRun{} }

type creativeRunData struct {
	Instruction string `json:"instruction"`
	// Activity 本轮「刚刚在做什么」的人话描述，供活动状态直接展示
	Activity string `json:"activity,omitempty"`
}

type creativeRun struct{ data creativeRunData }

func (r *creativeRun) Start(_ context.Context, env Env) error {
	positive := moodPositive(env.MoodLabel)
	requirement := strings.TrimSpace(env.Instruction)
	idea := requirement
	if idea == "" {
		idea = "延续之前的创作思路，挑一件合适的作品或项目继续推进；也可以发起全新的创作。"
	}
	idea = textutil.TruncateRunes(idea, 2000)
	focus := "可以展开的创作方向：" + idea + "。"
	if requirement != "" {
		focus = "本轮必须完成的要求（工具触发的强制指令）：" + idea + "。"
	}
	moodHint := "心情偏正向：沿着上一轮创作思路，把本轮愿望里的期望延伸下去。"
	if !positive {
		moodHint = "心情偏负面：可以无视与本轮无关的旧信息，自行规划创作方向。"
	}
	r.data.Instruction = fmt.Sprintf(
		"这是月华本人的当前状态（心情与生活记录都属于月华自己，不是用户的状态）："+
			"当前心情「%s」；刚才月华在：%s。%s\n"+
			"【当前愿望清单（可优先围绕与创作相关的愿望展开）】\n%s\n%s\n"+
			"请先阅读创作规划日志，再把本轮要做的事编排成有序计划并执行。",
		emptyAs(env.MoodLabel, "平静"), emptyAs(env.LastRecord, "没做什么"), focus,
		wishchain.FormatList(), moodHint,
	)
	return nil
}

func (r *creativeRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	result, err := creation.RunWithReferenceResult(ctx, r.data.Instruction, "")
	if err != nil {
		// 创作失败：命中相关愿望时直接判定逾期，不再等截止时间。
		wishchain.MarkOverdueMatching(result.Topic + "\n" + err.Error())
		// 创作过程问题接入心愿链路。
		wishchain.NoteLink(wishchain.LinkCreationIssue, truncateAtSentence(err.Error(), 120))
		return EventStep{}, fmt.Errorf("创作失败: %w", err)
	}
	summary := strings.TrimSpace(result.Summary)
	activity := strings.TrimSpace(result.Activity)
	if activity == "" {
		activity = "完成了一次创作"
	}
	if summary == "" {
		summary = "整理了一下创作思路，暂时没有落笔"
	}
	r.data.Activity = activity
	matchText := result.Topic + "\n" + activity + "\n" + summary
	if result.Failed {
		// 整轮没做成：相关愿望标记逾期。
		wishchain.MarkOverdueMatching(matchText)
		wishchain.NoteLink(wishchain.LinkCreationIssue, truncateAtSentence(activity, 120))
	} else {
		// 创作完成：命中相关愿望时标记完成；联动记录用活动文案。
		wishchain.CompleteMatching(matchText)
		wishchain.NoteLink(wishchain.LinkCreationDone, truncateAtSentence(activity, 120))
	}
	return EventStep{
		Done:   true,
		Record: activity,
		Detail: fmt.Sprintf("创作返回 %d 字", len([]rune(summary))),
	}, nil
}

// ActivityText 让活动状态优先展示一句人话，而不是机械的步骤摘要。
func (r *creativeRun) ActivityText() string {
	return strings.TrimSpace(r.data.Activity)
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

// moodPositive 判断心情是否偏正向；未知或空标签按正向处理。
func moodPositive(label string) bool {
	switch strings.TrimSpace(label) {
	case "愤怒", "伤心", "无聊", "失望", "厌倦", "担忧", "恐慌", "遗憾":
		return false
	}
	return true
}
