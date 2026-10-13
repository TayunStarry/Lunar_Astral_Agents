package enginebus

import (
	"encoding/json"
	"testing"
)

// fakeObserver 验证反射索引：仅按方法名约定，无需任何注册表
type fakeObserver struct {
	animations  int
	telemetries int
	lastRaw     []byte
}

func (o *fakeObserver) Name() string { return "fake" }

// OnAnimationList 对应消息 type "animation_list"
func (o *fakeObserver) OnAnimationList(raw []byte) {
	o.animations++
	o.lastRaw = raw
}

// OnTelemetry 对应消息 type "telemetry"
func (o *fakeObserver) OnTelemetry(raw []byte) {
	o.telemetries++
}

// 签名不符的方法不应被索引
func (o *fakeObserver) OnIgnored(value string) {}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"AnimationList": "animation_list",
		"Telemetry":     "telemetry",
		"AgentPosition": "agent_position",
		"LTP9Event":     "ltp9_event",
	}
	for in, want := range cases {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubscribeAndPublish(t *testing.T) {
	bus := NewBus()
	ob := &fakeObserver{}
	if n := bus.Subscribe(ob); n != 2 {
		t.Fatalf("应索引 2 个方法（OnIgnored 签名不符），实际 %d", n)
	}

	raw, _ := json.Marshal(map[string]any{"type": "animation_list", "payload": map[string]any{"a": 1}})
	observed := bus.Publish(raw)
	if len(observed) != 1 || observed[0] != "animation_list" {
		t.Fatalf("观察结果异常: %v", observed)
	}
	if ob.animations != 1 {
		t.Fatalf("OnAnimationList 未被调用: %d", ob.animations)
	}
	if string(ob.lastRaw) != string(raw) {
		t.Fatal("观察者未收到原始字节")
	}

	// 未注册的消息类型：不观测、不报错
	if got := bus.Publish([]byte(`{"type":"unknown_type"}`)); len(got) != 0 {
		t.Fatalf("未注册类型不应被观测: %v", got)
	}

	// 非法 JSON / 缺 type：跳过观测但不 panic（透传由调用方保证）
	if got := bus.Publish([]byte(`not-json`)); len(got) != 0 {
		t.Fatalf("非法 JSON 不应被观测: %v", got)
	}
	if got := bus.Publish([]byte(`{"payload":{}}`)); len(got) != 0 {
		t.Fatalf("缺 type 不应被观测: %v", got)
	}
}

// panicObserver 验证观测异常被捕获且不向外抛出
type panicObserver struct{}

func (o *panicObserver) Name() string           { return "panic" }
func (o *panicObserver) OnTelemetry(raw []byte) { panic("观测内部错误") }

func TestPublishRecoversPanic(t *testing.T) {
	bus := NewBus()
	bus.Subscribe(&panicObserver{})

	got := bus.Publish([]byte(`{"type":"telemetry","payload":{}}`))
	if len(got) != 0 {
		t.Fatalf("观测异常时不应计入已观测: %v", got)
	}
}

func TestEmit(t *testing.T) {
	bus := NewBus()
	if err := bus.Emit("animation_list", map[string]any{"a": 1}); err == nil {
		t.Fatal("未注入发送函数时应返回错误")
	}

	var sent []byte
	bus.SetSender(func(data []byte) { sent = data })
	if err := bus.Emit("telemetry", map[string]any{"x": 1}); err != nil {
		t.Fatalf("Emit 失败: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(sent, &env); err != nil {
		t.Fatalf("发送内容不是合法信封: %v", err)
	}
	if env.Type != "telemetry" || len(env.Payload) == 0 {
		t.Fatalf("信封内容异常: %+v", env)
	}
}
