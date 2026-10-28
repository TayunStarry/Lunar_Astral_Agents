package life

// event_watch.go 刷视频：生活事件只负责启动一轮哔哩观影，
// 待处理切片交给观影体系按中优先级逐片执行。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/agents/BilibiliViewer"
)

type watchEvent struct{ params eventParams }

func newWatchEvent() Event { return &watchEvent{params: paramsOf("watch")} }

func (e *watchEvent) ID() string       { return "watch" }
func (e *watchEvent) Weight() int      { return e.params.Weight }
func (e *watchEvent) DailyQuota() int  { return e.params.Quota }
func (e *watchEvent) NewRun() EventRun { return &watchRun{} }

type watchRunData struct {
	Mood string `json:"mood"`
}

type watchRun struct{ data watchRunData }

func (r *watchRun) Start(_ context.Context, env Env) error {
	r.data.Mood = emptyAs(env.MoodLabel, "平静")
	return nil
}

func (r *watchRun) Next(_ context.Context, _ Env) (EventStep, error) {
	text, err := BilibiliViewer.StartLifeRound(r.data.Mood)
	if err != nil {
		return EventStep{}, fmt.Errorf("启动哔哩观影失败: %w", err)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "挑了几个视频，准备慢慢看"
	}
	return EventStep{
		Done:   true,
		Record: "刷了会视频：" + truncateRunes(text, 200),
		Detail: text,
	}, nil
}

func (r *watchRun) Snapshot() json.RawMessage {
	raw, _ := json.Marshal(r.data)
	return raw
}

func (r *watchRun) Restore(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, &r.data)
}
