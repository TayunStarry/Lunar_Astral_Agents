package life

// event_storyboard.go 演绎事件：生活系统或月华命令触发后，下一 Tick 跳过权重选择，
// 直接把故事提交给演绎链；生活/命令触发不做人工审查，规划完成即开始分镜合成。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/chain/storyboard"
	"LunarSubsystem/LunarHarness/engine/activity"
	"LunarSubsystem/LunarHarness/internal/textutil"
)

type storyboardEvent struct{ params eventParams }

func newStoryboardEvent() Event { return &storyboardEvent{params: paramsOf("storyboard")} }

func (e *storyboardEvent) ID() string       { return "storyboard" }
func (e *storyboardEvent) Weight() int      { return e.params.Weight }
func (e *storyboardEvent) DailyQuota() int  { return e.params.Quota }
func (e *storyboardEvent) NewRun() EventRun { return &storyboardRun{} }

type storyboardRunData struct {
	Instruction string   `json:"instruction"`
	Handles     []string `json:"handles,omitempty"`
}

type storyboardRun struct {
	data storyboardRunData
}

func (r *storyboardRun) Start(_ context.Context, env Env) error {
	r.data.Instruction = strings.TrimSpace(env.Instruction)
	r.data.Handles = append([]string(nil), env.Handles...)
	return nil
}

func (r *storyboardRun) Next(_ context.Context, _ Env) (EventStep, error) {
	request := storyboard.SubmitRequest{ReviewRequired: false}
	if len(r.data.Handles) > 0 {
		request.Handle = r.data.Handles[0]
	} else {
		request.Text = r.data.Instruction
	}
	view, err := storyboard.SubmitTask(request)
	if err != nil {
		return EventStep{}, fmt.Errorf("演绎提交失败: %w", err)
	}
	title := strings.TrimSpace(view.Title)
	if title == "" {
		title = "一段故事"
	}
	summary := "开始演绎《" + title + "》，正在拆分分镜"
	activity.Set(summary)
	return EventStep{
		Done:   true,
		Record: textutil.TruncateRunes(summary, activityTextLimit),
		Detail: "演绎任务 " + view.TaskID,
	}, nil
}

func (r *storyboardRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *storyboardRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}
