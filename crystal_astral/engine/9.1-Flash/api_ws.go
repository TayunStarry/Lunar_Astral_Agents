package StarLTP

// ==== 全局 WebSocket 客户端（allow-network） ====
// 基于 gorilla/websocket，连接与读写都在 goroutine，所有 goja 回调（onopen/onmessage/onerror/onclose）
// 经 p.loop.RunOnLoop 排到插件自己的事件循环线程触发（保证 goja 线程安全）。
// 供插件作主动 WebSocket 客户端使用。

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"LunarSubsystem/LoggerGeneral"
	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
)

// wsReadyState WebSocket 连接状态常量（同浏览器）。
const (
	wsConnecting = 0
	wsOpen       = 1
	wsClosing    = 2
	wsClosed     = 3
)

// wsClient 一个 WebSocket 客户端实例。
type wsClient struct {
	p          *plugin
	vm         *goja.Runtime
	mu         sync.RWMutex
	conn       *websocket.Conn
	readyState int
	onopen     goja.Value
	onmessage  goja.Value
	onerror    goja.Value
	onclose    goja.Value
}

// bindWebSocket 把全局 WebSocket 构造器注册进插件沙箱。
func bindWebSocket(vm *goja.Runtime, p *plugin) {
	vm.Set("WebSocket", func(call goja.ConstructorCall) goja.Value {
		target := ""
		if len(call.Arguments) > 0 {
			target = call.Argument(0).String()
		}
		c := &wsClient{p: p, vm: vm, readyState: wsConnecting}
		obj := vm.NewObject()
		obj.Set("CONNECTING", wsConnecting)
		obj.Set("OPEN", wsOpen)
		obj.Set("CLOSING", wsClosing)
		obj.Set("CLOSED", wsClosed)
		// readyState 实时反映 Go 侧状态
		obj.DefineAccessorProperty("readyState",
			vm.ToValue(func(goja.FunctionCall) goja.Value {
				c.mu.RLock()
				defer c.mu.RUnlock()
				return vm.ToValue(c.readyState)
			}),
			goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_FALSE)
		obj.Set("send", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				c.jsSend(call.Argument(0).String())
			}
			return goja.Undefined()
		})
		obj.Set("close", func(goja.FunctionCall) goja.Value {
			c.jsClose()
			return goja.Undefined()
		})
		c.defineCallback(obj, "onopen", &c.onopen)
		c.defineCallback(obj, "onmessage", &c.onmessage)
		c.defineCallback(obj, "onerror", &c.onerror)
		c.defineCallback(obj, "onclose", &c.onclose)
		go c.run(target)
		return vm.ToValue(obj)
	})
}

// defineCallback 为某回调属性定义 getter/setter，把 JS 赋值存取到 Go 字段。
func (c *wsClient) defineCallback(obj *goja.Object, name string, store *goja.Value) {
	getter := func(goja.FunctionCall) goja.Value {
		c.mu.RLock()
		defer c.mu.RUnlock()
		if *store == nil {
			return goja.Undefined()
		}
		return *store
	}
	setter := func(call goja.FunctionCall) {
		c.mu.Lock()
		defer c.mu.Unlock()
		*store = call.Argument(0)
	}
	obj.DefineAccessorProperty(name, c.vm.ToValue(getter), c.vm.ToValue(setter), goja.FLAG_FALSE, goja.FLAG_TRUE)
}

// run 在 goroutine 里建立连接并进入读循环。
func (c *wsClient) run(target string) {
	u, err := url.Parse(target)
	if err != nil {
		c.fireError(fmt.Errorf("WebSocket url 解析失败: %v", err))
		return
	}
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		c.fireError(err)
		return
	}
	c.mu.Lock()
	c.conn = conn
	c.readyState = wsOpen
	openCB := c.onopen
	c.mu.Unlock()
	if openCB != nil {
		c.onLoop(func(vm *goja.Runtime) {
			if f, ok := goja.AssertFunction(openCB); ok {
				f(goja.Undefined())
			}
		})
	}
	c.readLoop(conn)
}

func (c *wsClient) readLoop(conn *websocket.Conn) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.readyState = wsClosed
			c.conn = nil
			closeCB := c.onclose
			c.mu.Unlock()
			if closeCB != nil {
				c.onLoop(func(vm *goja.Runtime) {
					if f, ok := goja.AssertFunction(closeCB); ok {
						f(goja.Undefined())
					}
				})
			}
			conn.Close()
			return
		}
		c.mu.RLock()
		cb := c.onmessage
		c.mu.RUnlock()
		if cb != nil {
			payload := string(data)
			c.onLoop(func(vm *goja.Runtime) {
				f, ok := goja.AssertFunction(cb)
				if !ok {
					return
				}
				ev := vm.NewObject()
				ev.Set("data", vm.ToValue(payload))
				f(goja.Undefined(), ev)
			})
		}
	}
}

// fireError 触发 onerror（并在未设置时记日志）。
func (c *wsClient) fireError(err error) {
	c.mu.RLock()
	cb := c.onerror
	c.mu.RUnlock()
	if cb == nil {
		LoggerGeneral.Warn(ServiceName, "插件 WebSocket 错误: %v", err)
		// 错误通常伴随断连，补发 onclose
		c.mu.Lock()
		closeCB := c.onclose
		c.readyState = wsClosed
		c.mu.Unlock()
		if closeCB != nil {
			c.onLoop(func(vm *goja.Runtime) {
				if f, ok := goja.AssertFunction(closeCB); ok {
					f(goja.Undefined())
				}
			})
		}
		return
	}
	msg := err.Error()
	c.onLoop(func(vm *goja.Runtime) {
		f, ok := goja.AssertFunction(cb)
		if !ok {
			return
		}
		ev := vm.NewObject()
		ev.Set("error", vm.ToValue(msg))
		f(goja.Undefined(), ev)
	})
}

// jsSend 发送文本（写是 goroutine-safe）。
func (c *wsClient) jsSend(data string) {
	c.mu.RLock()
	conn := c.conn
	ready := c.readyState
	c.mu.RUnlock()
	if conn == nil || ready != wsOpen {
		return
	}
	_ = conn.WriteMessage(websocket.TextMessage, []byte(data))
}

// jsClose 关闭连接。
func (c *wsClient) jsClose() {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
}

// onLoop 在插件事件循环上执行回调。
func (c *wsClient) onLoop(fn func(vm *goja.Runtime)) {
	if c.p == nil || c.p.loop == nil {
		return
	}
	c.p.loop.RunOnLoop(fn)
}
