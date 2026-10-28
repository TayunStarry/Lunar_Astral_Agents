package life

// event_daze.go 发呆：默认兜底事件，不调用模型、不访问网络。

import (
	"context"
	"encoding/json"
)

type dazeEvent struct{ params eventParams }

func newDazeEvent() Event { return &dazeEvent{params: paramsOf("daze")} }

func (e *dazeEvent) ID() string       { return "daze" }
func (e *dazeEvent) Weight() int      { return e.params.Weight }
func (e *dazeEvent) DailyQuota() int  { return e.params.Quota }
func (e *dazeEvent) NewRun() EventRun { return &dazeRun{} }

type dazeRun struct{}

func (r *dazeRun) Start(context.Context, Env) error { return nil }

func (r *dazeRun) Next(context.Context, Env) (EventStep, error) {
	return EventStep{Done: true, Record: "发呆中", Detail: "发了一会儿呆"}, nil
}

func (r *dazeRun) Snapshot() json.RawMessage { return json.RawMessage("{}") }

func (r *dazeRun) Restore(json.RawMessage) error { return nil }
