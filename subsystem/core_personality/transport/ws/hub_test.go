package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ==== 通道分离契约测试 ====
//
// 背景：合并为单进程后，引擎/工作室模块经 /write/engine 高频互通原始帧，
// 而消息终端前端会把无法识别的帧当作聊天内容渲染。因此原始帧必须只投递给
// 显式订阅（/ws?bus=1）的连接，结构化消息仍投递给全部连接。

// newTestHub 启动测试用集线器与 HTTP 服务
func newTestHub(t *testing.T) (*Hub, *httptest.Server) {
	t.Helper()
	hub := NewHub()
	go hub.Run()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	t.Cleanup(func() {
		srv.Close()
		hub.CloseAll()
	})
	return hub, srv
}

// dial 建立一条 WS 连接；query 形如 "?bus=1"，空串表示不声明总线订阅
func dial(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + query
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接 %s 失败: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// waitClients 等待集线器注册到指定连接数
func waitClients(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if hub.ClientCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("等待 %d 个连接注册超时（当前 %d）", want, hub.ClientCount())
}

// readWithTimeout 读取一条消息；超时返回 nil
func readWithTimeout(conn *websocket.Conn, d time.Duration) []byte {
	_ = conn.SetReadDeadline(time.Now().Add(d))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil
	}
	return data
}

// TestBroadcastBusOnlyReachesSubscribers 原始总线帧只投递给显式订阅者
func TestBroadcastBusOnlyReachesSubscribers(t *testing.T) {
	hub, srv := newTestHub(t)

	subscribed := dial(t, srv, "?"+BusParam+"=1")
	plain := dial(t, srv, "")

	waitClients(t, hub, 2)
	if n := hub.BusCount(); n != 1 {
		t.Fatalf("BusCount 应为 1（仅 ?bus=1 的连接订阅），实际 %d", n)
	}

	frame := `{"type":"telemetry","payload":{"character":{"x":1}}}`
	hub.BroadcastBus(DefaultBusChannel, []byte(frame))

	if got := readWithTimeout(subscribed, 2*time.Second); string(got) != frame {
		t.Fatalf("订阅者未收到总线帧: %q", got)
	}
	if got := readWithTimeout(plain, 500*time.Millisecond); got != nil {
		t.Fatalf("未订阅者收到总线帧（消息终端将被污染）: %q", got)
	}
}

// TestBroadcastBusIsolatesChannels 总线帧只投递给**同频道**的订阅者。
// 对应缺陷：同一进程内两个独立引擎实例（月华内嵌渲染引擎 / 琉璃 studio 编辑器引擎）
// 若不按频道隔离，各自的 10Hz 遥测会互相串扰，表现为 studio 信息栏高频刷动。
func TestBroadcastBusIsolatesChannels(t *testing.T) {
	hub, srv := newTestHub(t)

	// 每组断言使用独立连接：gorilla 连接在读超时后即损坏，不可复用；
	// 第二组连接在第一条帧广播之后建立，避免其队列里残留上一次的帧
	studioA := dial(t, srv, "?"+BusParam+"=1&"+ChannelParam+"=lunar-engine-bus")
	lunarA := dial(t, srv, "?"+BusParam+"=1&"+ChannelParam+"=message-terminal-renderer")
	waitClients(t, hub, 2)

	// 月华内嵌引擎的遥测：只应留在自己的频道，studio 侧不该看到
	lunarFrame := `{"type":"telemetry","source":"engine","payload":{"fps":60}}`
	hub.BroadcastBus("message-terminal-renderer", []byte(lunarFrame))

	if got := readWithTimeout(lunarA, 2*time.Second); string(got) != lunarFrame {
		t.Fatalf("本频道订阅者未收到帧: %q", got)
	}
	if got := readWithTimeout(studioA, 500*time.Millisecond); got != nil {
		t.Fatalf("跨频道串扰：studio 收到了月华引擎的帧: %q", got)
	}

	studioB := dial(t, srv, "?"+BusParam+"=1&"+ChannelParam+"=lunar-engine-bus")
	lunarB := dial(t, srv, "?"+BusParam+"=1&"+ChannelParam+"=message-terminal-renderer")
	waitClients(t, hub, 4)
	if n := hub.BusCount(); n != 4 {
		t.Fatalf("BusCount 应为 4，实际 %d", n)
	}

	// studio 频道的帧仍应送达 studio，且不回流到月华引擎
	studioFrame := `{"type":"mode_changed","payload":{"mode":"editor"}}`
	hub.BroadcastBus("lunar-engine-bus", []byte(studioFrame))

	if got := readWithTimeout(studioB, 2*time.Second); string(got) != studioFrame {
		t.Fatalf("studio 未收到本频道帧: %q", got)
	}
	if got := readWithTimeout(lunarB, 500*time.Millisecond); got != nil {
		t.Fatalf("跨频道串扰：月华引擎收到了 studio 频道的帧: %q", got)
	}
}

// TestRequestChannelDefaulting 频道声明解析：未声明时归入默认频道
func TestRequestChannelDefaulting(t *testing.T) {
	cases := map[string]string{
		"/ws":                                    DefaultBusChannel,
		"/ws?bus=1":                              DefaultBusChannel,
		"/ws?channel=message-terminal-renderer":  "message-terminal-renderer",
		"/write/engine?channel=lunar-engine-bus": "lunar-engine-bus",
		"/write/engine?channel=%20%20":           DefaultBusChannel,
	}
	for target, want := range cases {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if got := RequestChannel(req); got != want {
			t.Errorf("RequestChannel(%q) = %q, want %q", target, got, want)
		}
	}
}

// TestBroadcastMessageReachesEveryone 结构化消息仍投递给全部连接
func TestBroadcastMessageReachesEveryone(t *testing.T) {
	hub, srv := newTestHub(t)

	subscribed := dial(t, srv, "?"+BusParam+"=1")
	plain := dial(t, srv, "")
	waitClients(t, hub, 2)

	hub.BroadcastMessage("engine", map[string]any{"type": "action", "payload": map[string]any{"action": "wave"}})

	for name, conn := range map[string]*websocket.Conn{"订阅者": subscribed, "非订阅者": plain} {
		got := readWithTimeout(conn, 2*time.Second)
		if got == nil {
			t.Fatalf("%s 未收到结构化消息", name)
		}
		if !strings.Contains(string(got), `"type":"engine"`) {
			t.Fatalf("%s 收到异常内容: %s", name, got)
		}
	}
}

// TestSubscribeBusParsing 订阅声明解析：仅显式真值订阅
func TestSubscribeBusParsing(t *testing.T) {
	cases := map[string]bool{
		"": false, "0": false, "false": false, "no": false, "off": false, "random": false,
		"1": true, "true": true, "TRUE": true, "yes": true, "on": true, "%201%20": true,
	}
	for value, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/ws?bus="+value, nil)
		if got := subscribeBus(req); got != want {
			t.Errorf("subscribeBus(bus=%q) = %v, want %v", value, got, want)
		}
	}
	// 完全不带参数时默认不订阅
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if subscribeBus(req) {
		t.Error("缺省连接不应订阅引擎总线（消息终端依赖此默认）")
	}
}

// TestChannelBusCount 频道订阅计数：缺省频道与具名频道分开统计。
// LTPX 前端守护以「缺省频道订阅数 > 0」判定琉璃桌面是否在线——
// 琉璃桌面以 /ws?bus=1 接入，未订阅总线（bus=false）的连接不计入。
func TestChannelBusCount(t *testing.T) {
	hub, srv := newTestHub(t)

	crystalDesktop := dial(t, srv, "?"+BusParam+"=1") // 琉璃桌面式连接（缺省频道）
	named := dial(t, srv, "?"+BusParam+"=1&"+ChannelParam+"=message-terminal-renderer")
	plainSameChannel := dial(t, srv, "") // 未订阅总线，但频道解析同为缺省频道
	waitClients(t, hub, 3)

	if n := hub.ChannelBusCount(DefaultBusChannel); n != 1 {
		t.Fatalf("缺省频道订阅数应为 1（未订阅总线的连接不计入），实际 %d", n)
	}
	if n := hub.ChannelBusCount("message-terminal-renderer"); n != 1 {
		t.Fatalf("具名频道订阅数应为 1，实际 %d", n)
	}
	if n := hub.ChannelBusCount("unknown-channel"); n != 0 {
		t.Fatalf("未知频道订阅数应为 0，实际 %d", n)
	}
	if n := hub.ChannelBusCount(""); n != 1 {
		t.Fatalf("空频道应按缺省频道统计，实际 %d", n)
	}

	_ = crystalDesktop
	_ = named
	_ = plainSameChannel
}

// TestChannelClientCount 频道连接计数：不区分是否订阅总线。
// 月华消息终端以 /ws?channel=message-terminal 接入且未订阅总线，
// 控制台在线探针须以「频道连接数」而非「总线订阅数」判定其存活。
func TestChannelClientCount(t *testing.T) {
	hub, srv := newTestHub(t)

	busClient := dial(t, srv, "?"+BusParam+"=1") // 琉璃桌面式连接（缺省频道 + 总线订阅）
	lunarTerminal := dial(t, srv, "?"+ChannelParam+"=message-terminal")
	lunarMobile := dial(t, srv, "?"+ChannelParam+"=message-terminal")
	waitClients(t, hub, 3)

	if n := hub.ChannelClientCount("message-terminal"); n != 2 {
		t.Fatalf("月华终端频道连接数应为 2，实际 %d", n)
	}
	if n := hub.ChannelClientCount(DefaultBusChannel); n != 1 {
		t.Fatalf("缺省频道连接数应为 1，实际 %d", n)
	}
	if n := hub.ChannelBusCount("message-terminal"); n != 0 {
		t.Fatalf("未订阅总线的终端不应计入总线订阅数，实际 %d", n)
	}
	if n := hub.ChannelClientCount(""); n != 1 {
		t.Fatalf("空频道应按缺省频道统计，实际 %d", n)
	}

	_ = busClient
	_ = lunarTerminal
	_ = lunarMobile
}
