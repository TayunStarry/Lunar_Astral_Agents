package life

// event_creative.go 创作事件：生活体系触发一轮「月华.生活.创作」管线。
// 创作参考来自 life_planning 写入的字符串变量，不再截取历史对话；
// 心情正负决定是延续上一轮思路，还是允许自行规划。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/chain/creation"
)

type creativeEvent struct{ params eventParams }

func newCreativeEvent() Event { return &creativeEvent{params: paramsOf("creative")} }

func (e *creativeEvent) ID() string       { return "creative" }
func (e *creativeEvent) Weight() int      { return e.params.Weight }
func (e *creativeEvent) DailyQuota() int  { return e.params.Quota }
func (e *creativeEvent) NewRun() EventRun { return &creativeRun{} }

type creativeRunData struct {
	Instruction string `json:"instruction"`
	Reference   string `json:"reference,omitempty"`
}

type creativeRun struct{ data creativeRunData }

func (r *creativeRun) Start(_ context.Context, env Env) error {
	reference := CreativeIntent()
	referenceText := truncateRunes(reference, 2000)
	positive := moodPositive(env.MoodLabel)
	requirement := strings.TrimSpace(env.Instruction)
	idea := requirement
	if idea == "" {
		idea = "延续之前的创作思路，挑一件合适的作品或项目继续推进；也可以发起全新的创作。"
	}
	idea = truncateRunes(idea, 2000)
	focus := "可以展开的创作方向：" + idea + "。"
	if requirement != "" {
		focus = "本轮必须完成的要求（工具触发的强制指令）：" + idea + "。"
	}
	if referenceText == "" {
		referenceText = "（本轮没有新的创作参考，按上一轮思路或自由发挥）"
	}
	moodHint := "心情偏正向：沿着上一轮创作思路，把本轮创作参考里的期望延伸下去。"
	if !positive {
		moodHint = "心情偏负面：可以无视与本轮无关的旧信息，自行规划创作方向。"
	}
	r.data.Reference = reference
	r.data.Instruction = fmt.Sprintf(
		"这是月华本人的当前状态（心情与生活记录都属于月华自己，不是用户的状态）："+
			"当前心情「%s」；刚才月华在：%s。%s\n"+
			"【本轮创作参考】%s\n%s\n"+
			"请先阅读创作规划日志，再把本轮要做的事编排成有序计划并执行。",
		emptyAs(env.MoodLabel, "平静"), emptyAs(env.LastRecord, "没做什么"), focus,
		referenceText, moodHint,
	)
	return nil
}

func (r *creativeRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	reference := r.data.Reference
	text, err := creation.RunWithReference(ctx, r.data.Instruction, reference)
	// 一轮操作结束：只重置本轮消费掉的值，避免覆盖运行期间新写入的意图
	defer ResetCreativeIntentIf(reference)
	if err != nil {
		return EventStep{}, fmt.Errorf("创作失败: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "整理了一下创作思路，暂时没有落笔"
	}
	return EventStep{
		Done:   true,
		Record: "创作了：" + truncateAtSentence(text, 512),
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

// moodPositive 判断心情是否偏正向；未知或空标签按正向处理。
func moodPositive(label string) bool {
	switch strings.TrimSpace(label) {
	case "愤怒", "伤心", "无聊", "厌倦", "担忧", "恐慌":
		return false
	}
	return true
}
