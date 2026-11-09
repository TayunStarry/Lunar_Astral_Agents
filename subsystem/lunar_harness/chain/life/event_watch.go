package life

// event_watch.go 看视频分支：取一个已下载未看的片段，理解画面、留观感评论、更新状态。
//
// 生活链权重分支与对话者工具触发都落到这一个事件，事件内部调用
// viewer.WatchNextSegment，不复制第二份实现。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"LunarSubsystem/LunarHarness/chain/viewer"
	"LunarSubsystem/LunarHarness/chain/wishchain"
)

type watchEvent struct{ params eventParams }

func newWatchEvent() Event { return &watchEvent{params: paramsOf("watch")} }

func (e *watchEvent) ID() string       { return "watch" }
func (e *watchEvent) Weight() int      { return e.params.Weight }
func (e *watchEvent) DailyQuota() int  { return e.params.Quota }
func (e *watchEvent) NewRun() EventRun { return &watchRun{} }

type watchRunData struct {
	Instruction string `json:"instruction"`
}

type watchRun struct {
	data    watchRunData
	outcome viewer.WatchOutcome
}

func (r *watchRun) Start(_ context.Context, env Env) error {
	r.data.Instruction = strings.TrimSpace(env.Instruction)
	return nil
}

func (r *watchRun) Next(ctx context.Context, _ Env) (EventStep, error) {
	outcome, err := watchStepHook(ctx, r.data.Instruction)
	if err != nil {
		// 观看过程失败：命中相关愿望时直接判定逾期。
		wishchain.MarkOverdueMatching(strings.Join([]string{r.data.Instruction, err.Error()}, "\n"))
		return EventStep{}, err
	}
	r.outcome = outcome
	matchText := watchMatchText(outcome, r.data.Instruction)
	if outcome.Triggered && outcome.LastSegment {
		// 整支视频看完：命中相关愿望时标记完成。
		wishchain.CompleteMatching(matchText)
	} else if outcome.Failed {
		// 无片可看或视频已不在列表：命中相关愿望时标记逾期。
		wishchain.MarkOverdueMatching(matchText + "\n" + outcome.Reason)
	}
	return EventStep{
		Done:   true,
		Record: outcome.RecordText(),
		Detail: watchDetail(outcome),
	}, nil
}

// watchMatchText 拼接用于愿望匹配的文本：对话者要求 + 视频标题 + 印象主题词。
func watchMatchText(outcome viewer.WatchOutcome, instruction string) string {
	parts := make([]string, 0, len(outcome.Topics)+2)
	parts = append(parts, instruction, outcome.Title)
	parts = append(parts, outcome.Topics...)
	return strings.Join(parts, "\n")
}

// ActivityText 看视频分支已在 viewer 内部写过具体文案：有片段时原样保留，
// 没看成时补一句「想看点视频」的早退说明，避免兜底文案只剩一句原因。
func (r *watchRun) ActivityText() string {
	if r.outcome.Triggered {
		return r.outcome.Activity
	}
	return "想看点视频，" + emptyAs(r.outcome.Reason, "但没有可看的片段")
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

// watchDetail 把一次观看的关键结果压成一行，便于观测。
func watchDetail(outcome viewer.WatchOutcome) string {
	if !outcome.Triggered {
		return emptyAs(outcome.Reason, "没有看片段")
	}
	parts := []string{
		fmt.Sprintf("%s 第 %d/%d 段", outcome.BV, outcome.SegmentIndex, outcome.SegmentTotal),
		"画面=" + outcome.VisualSource,
	}
	if outcome.CommentPosted {
		parts = append(parts, "评论已发")
	} else {
		parts = append(parts, "评论未发")
	}
	if outcome.Mentioned {
		parts = append(parts, "已@特别关注")
	}
	if outcome.DynamicPosted {
		parts = append(parts, "阶段动态已发")
	}
	if outcome.Note != "" {
		parts = append(parts, outcome.Note)
	}
	return strings.Join(parts, "，")
}
