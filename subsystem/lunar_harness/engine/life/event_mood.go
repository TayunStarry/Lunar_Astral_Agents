package life

// event_mood.go 更新心情：唯一允许触碰心情状态的生活事件。

import (
	"context"
	"encoding/json"

	"LunarSubsystem/LunarHarness/engine/mood"
)

type moodEvent struct{ params eventParams }

func newMoodEvent() Event { return &moodEvent{params: paramsOf("mood")} }

func (e *moodEvent) ID() string       { return "mood" }
func (e *moodEvent) Weight() int      { return e.params.Weight }
func (e *moodEvent) DailyQuota() int  { return e.params.Quota }
func (e *moodEvent) NewRun() EventRun { return &moodRun{} }

type moodRunData struct {
	Text string `json:"text"`
}

type moodRun struct{ data moodRunData }

func (r *moodRun) Start(_ context.Context, env Env) error {
	r.data.Text = env.LastUserText
	return nil
}

func (r *moodRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	if r.data.Text == "" {
		return EventStep{Done: true, Record: "出门转了转", Detail: "没有最新发言，保持当前心情"}, nil
	}
	before := mood.Snapshot().Label
	if err := mood.UpdateFromText(ctx, r.data.Text); err != nil {
		return EventStep{}, err
	}
	after := mood.Snapshot().Label
	return EventStep{
		Done:   true,
		Record: "出门转了转",
		Detail: "心情 " + before + " → " + after,
	}, nil
}

func (r *moodRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *moodRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}
