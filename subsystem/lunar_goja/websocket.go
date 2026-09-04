package LunarGoja

import (
	"LunarSubsystem/LoggerGeneral"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/gorilla/websocket"
)

// ==== WebSocket（标准 WebSocket 客户端） ====

// WebSocket 就绪状态常量（与浏览器 WebSocket.readyState 取值一致）
const (
	wsConnecting = 0 // CONNECTING 正在建立连接
	wsOpen       = 1 // OPEN 连接已建立，可收发消息
	wsClosing    = 2 // CLOSING 关闭握手中
	wsClosed     = 3 // CLOSED 连接已关闭或建立失败
)

// wsConnection 单个 WebSocket 连接的内部状态
// 供 send/close 与读写 goroutine 共享，mu 保护 readyState 与 done
type wsConnection struct {
	runtime    *goja.Runtime
	obj        *goja.Object
	conn       *websocket.Conn
	mu         sync.Mutex
	readyState int
	done       bool // finish 是否已执行（保证 onclose 只触发一次）
}

// registerWebSocket 注册 WebSocket 构造函数并挂载 CONNECTING/OPEN/CLOSING/CLOSED 静态常量
func (env *standardEnv) registerWebSocket() {
	vm := env.runtime
	ctorValue := vm.ToValue(env.websocketConstructor)
	ctorObj := ctorValue.ToObject(vm)
	ctorObj.Set("CONNECTING", wsConnecting)
	ctorObj.Set("OPEN", wsOpen)
	ctorObj.Set("CLOSING", wsClosing)
	ctorObj.Set("CLOSED", wsClosed)
	vm.Set("WebSocket", ctorValue)
}

// websocketConstructor WebSocket 构造函数（new WebSocket(url)）
// 事件属性: onopen / onmessage(event.data) / onerror(event) / onclose(event)
// 方法: send(data) / close(code?, reason?)
// 只读属性: url / readyState / protocol / bufferedAmount
// 非法参数同步抛 TypeError；连接失败异步触发 onerror + onclose
func (env *standardEnv) websocketConstructor(call goja.ConstructorCall) *goja.Object {
	runtime := env.runtime
	this := call.This

	if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
		panic(runtime.NewTypeError("WebSocket: url 参数缺失"))
	}
	urlStr := call.Argument(0).String()
	parsed, err := url.Parse(urlStr)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		panic(runtime.NewTypeError("WebSocket: 仅支持 ws:// 或 wss:// 协议，收到: %s", urlStr))
	}

	state := &wsConnection{runtime: runtime, obj: this, readyState: wsConnecting}

	// 初始化公开属性与事件占位
	this.Set("url", urlStr)
	this.Set("readyState", wsConnecting)
	this.Set("protocol", "")
	this.Set("bufferedAmount", 0)
	this.Set("onopen", goja.Undefined())
	this.Set("onmessage", goja.Undefined())
	this.Set("onerror", goja.Undefined())
	this.Set("onclose", goja.Undefined())

	// send(data)：发送文本消息；连接未就绪或参数缺失时静默忽略（标准行为）
	this.Set("send", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		state.mu.Lock()
		if state.readyState != wsOpen || state.conn == nil {
			state.mu.Unlock()
			return goja.Undefined()
		}
		conn := state.conn
		state.mu.Unlock()

		if err := conn.WriteMessage(websocket.TextMessage, []byte(call.Argument(0).String())); err != nil {
			LoggerGeneral.Error("LunarGoja", "WebSocket 发送失败: %v", err)
			state.handleError(fmt.Errorf("发送失败: %v", err))
		}
		return goja.Undefined()
	})

	// close(code?, reason?)：发起关闭握手并断开连接，随后触发一次 onclose
	this.Set("close", func(call goja.FunctionCall) goja.Value {
		code := websocket.CloseNormalClosure
		if len(call.Arguments) > 0 {
			code = int(call.Argument(0).ToInteger())
		}
		reason := ""
		if len(call.Arguments) > 1 {
			reason = call.Argument(1).String()
		}

		// 先收敛连接状态（done 置位），确保由 close() 主导 onclose，
		// 避免读循环断开时的异常路径抢先触发
		state.mu.Lock()
		if state.done {
			state.mu.Unlock()
			return goja.Undefined()
		}
		state.done = true
		state.readyState = wsClosed
		conn := state.conn
		state.conn = nil
		state.mu.Unlock()

		RunOnLoop(func(vm *goja.Runtime) {
			state.obj.Set("readyState", wsClosed)
			state.dispatchEventOnLoop("onclose", state.eventObject("close", map[string]any{
				"code":     code,
				"reason":   reason,
				"wasClean": true,
			}))
		})

		if conn != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
			conn.Close()
		}
		return goja.Undefined()
	})

	// 异步建立连接，避免阻塞事件循环
	go state.dial(urlStr)
	return nil
}

// dial 在独立 goroutine 中建立 WebSocket 连接
func (s *wsConnection) dial(urlStr string) {
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, resp, err := dialer.Dial(urlStr, nil)
	if err != nil {
		LoggerGeneral.Error("LunarGoja", "WebSocket 连接失败(%s): %v", urlStr, err)
		s.handleError(fmt.Errorf("连接失败: %v", err))
		return
	}

	s.mu.Lock()
	if s.done {
		// 连接建立前已被 close()，直接释放
		s.mu.Unlock()
		conn.Close()
		return
	}
	s.conn = conn
	s.readyState = wsOpen
	s.mu.Unlock()

	// 回事件循环触发 onopen 并同步协议/就绪状态
	// 若期间已被 close() 收敛（done 置位），则不再触发 onopen
	RunOnLoop(func(vm *goja.Runtime) {
		s.mu.Lock()
		closed := s.done
		s.mu.Unlock()
		if closed {
			return
		}
		if resp != nil {
			s.obj.Set("protocol", resp.Header.Get("Sec-WebSocket-Protocol"))
		}
		s.obj.Set("readyState", wsOpen)
		s.dispatchEventOnLoop("onopen", s.eventObject("open", nil))
	})

	s.readLoop()
}

// readLoop 持续读取服务端消息，连接断开时收敛并触发 onclose
func (s *wsConnection) readLoop() {
	for {
		s.mu.Lock()
		conn := s.conn
		s.mu.Unlock()
		if conn == nil {
			return
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				s.finish(closeErr.Code, closeErr.Text, true)
			} else {
				s.finish(websocket.CloseAbnormalClosure, "连接异常断开: "+err.Error(), false)
			}
			return
		}

		switch msgType {
		case websocket.TextMessage:
			// 文本消息以 string 形式暴露
			s.dispatchEvent("onmessage", func(vm *goja.Runtime) *goja.Object {
				return s.eventObject("message", map[string]any{"data": string(data)})
			})
		case websocket.BinaryMessage:
			// 二进制消息以 ArrayBuffer 形式暴露
			s.dispatchEvent("onmessage", func(vm *goja.Runtime) *goja.Object {
				return s.eventObject("message", map[string]any{"data": vm.NewArrayBuffer(data)})
			})
		}
	}
}

// dispatchEvent 在事件循环上触发事件处理器（可从任意 goroutine 调用）
func (s *wsConnection) dispatchEvent(eventName string, buildEvent func(vm *goja.Runtime) *goja.Object) {
	RunOnLoop(func(vm *goja.Runtime) {
		s.dispatchEventOnLoop(eventName, buildEvent(vm))
	})
}

// dispatchEventOnLoop 触发事件处理器（必须在事件循环线程上调用）
func (s *wsConnection) dispatchEventOnLoop(eventName string, event *goja.Object) {
	handler := s.obj.Get(eventName)
	if fn, ok := goja.AssertFunction(handler); ok {
		fn(s.obj, event)
	}
}

// eventObject 构建事件对象（必须在事件循环线程上调用）
func (s *wsConnection) eventObject(eventType string, extra map[string]any) *goja.Object {
	obj := s.runtime.NewObject()
	obj.Set("type", eventType)
	obj.Set("target", s.obj)
	for key, value := range extra {
		obj.Set(key, value)
	}
	return obj
}

// handleError 连接失败/发送失败时触发 onerror 并收敛连接
func (s *wsConnection) handleError(err error) {
	RunOnLoop(func(vm *goja.Runtime) {
		s.dispatchEventOnLoop("onerror", s.eventObject("error", map[string]any{
			"message": err.Error(),
			"error":   err.Error(),
		}))
	})
	s.finish(websocket.CloseAbnormalClosure, err.Error(), false)
}

// finish 收敛连接：置 CLOSED 状态并触发一次 onclose
func (s *wsConnection) finish(code int, reason string, clean bool) {
	s.mu.Lock()
	if s.done {
		s.mu.Unlock()
		return
	}
	s.done = true
	s.readyState = wsClosed
	s.conn = nil
	s.mu.Unlock()

	RunOnLoop(func(vm *goja.Runtime) {
		s.obj.Set("readyState", wsClosed)
		s.dispatchEventOnLoop("onclose", s.eventObject("close", map[string]any{
			"code":     code,
			"reason":   reason,
			"wasClean": clean,
		}))
	})
}
