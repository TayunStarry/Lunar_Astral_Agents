package BrowserClient

// WebView 会话管理器：后端可驱动的多实例 webview 窗口（深读代理的基础设施）。
//
// 与 StartWebViewBrowser 的「单实例、启动即阻塞」模型不同，每个会话在独立 OS 线程上
// 创建并运行自己的 webview 实例，注册表持有引用供 HTTP 端点跨线程驱动：
//   - Navigate / EvalJSON 经 Dispatch 投递到会话的 UI 线程执行
//   - EvalJSON 的返回值经 Bind 注入的 __lunarReport 回调回传（UI 线程 → 缓冲通道 → 调用方）
//   - 用户直接关闭窗口时 Run 返回，会话自动注销并释放资源

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"LunarSubsystem/LoggerGeneral"

	webview "github.com/webview/webview_go"
)

// WebViewSession 一个后端可驱动的 webview 会话实例
type WebViewSession struct {
	ID    string
	Title string

	wv        webview.WebView
	navigateMu sync.Mutex // 串行化 EvalJSON / Navigate，避免结果通道串扰

	resultMu   sync.Mutex
	resultCh   chan string // 最近一次 Eval 的结果回传通道（__lunarReport 回调写入）
	closedCh   chan struct{}
	closeOnce  sync.Once
	aliveFlag  int32
	unregister func()
}

// 会话注册表
var (
	sessionRegistryMu  sync.Mutex
	sessionRegistry    = map[string]*WebViewSession{}
	sessionRegistryNew string // 最近创建的会话 ID（深读代理默认复用）
)

// PageState 页面就绪状态快照
type PageState struct {
	ReadyState string `json:"rs"`
	URL        string `json:"url"`
	Title      string `json:"title"`
}

// OpenWebViewSession 创建并注册一个新的 webview 会话（url 为空则停留在 about:blank）。
// 返回的会话已就绪可被驱动；用户关闭窗口后会话自动注销。
func OpenWebViewSession(url string, title string, width int, height int) (*WebViewSession, error) {
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 860
	}
	if title == "" {
		title = "琉璃 · 深读代理"
	}

	s := &WebViewSession{
		ID:       fmt.Sprintf("wv-%d", time.Now().UnixNano()%1_000_000),
		Title:    title,
		resultCh: make(chan string, 1),
		closedCh: make(chan struct{}),
	}
	created := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		w := webview.New(false)
		if w == nil {
			LoggerGeneral.SubError("BrowserClient", "OpenWebViewSession", "创建 webview 失败")
			created <- errors.New("创建 webview 失败")
			return
		}
		s.wv = w
		w.SetTitle(title)
		w.SetSize(width, height, webview.HintNone)

		// 结果回传绑定：Eval 注入的脚本通过 __lunarReport 把结果送回 Go（UI 线程回调，只做非阻塞投递）
		_ = w.Bind("__lunarReport", func(args ...interface{}) interface{} {
			if len(args) > 0 {
				if str, ok := args[0].(string); ok {
					s.resultMu.Lock()
					ch := s.resultCh
					s.resultMu.Unlock()
					if ch != nil {
						select {
						case ch <- str:
						default:
						}
					}
				}
			}
			return nil
		})

		if url != "" {
			w.Navigate(url)
		}
		atomic.StoreInt32(&s.aliveFlag, 1)
		created <- nil
		w.Run()

		// Run 返回：窗口已被关闭（用户或 Close），释放并注销
		w.Destroy()
		atomic.StoreInt32(&s.aliveFlag, 0)
		closeOnceDo(s)
	}()

	if err := <-created; err != nil {
		return nil, err
	}

	sessionRegistryMu.Lock()
	sessionRegistry[s.ID] = s
	sessionRegistryNew = s.ID
	sessionRegistryMu.Unlock()

	s.unregister = func() {
		sessionRegistryMu.Lock()
		delete(sessionRegistry, s.ID)
		if sessionRegistryNew == s.ID {
			sessionRegistryNew = ""
		}
		sessionRegistryMu.Unlock()
	}

	LoggerGeneral.SubInfo("BrowserClient", "OpenWebViewSession", "会话已创建 id=%s url=%s", s.ID, url)
	return s, nil
}

// closeOnceDo Run 返回后的统一清理（去重：Close 触发与用户关窗可能竞争）
func closeOnceDo(s *WebViewSession) {
	s.closeOnce.Do(func() {
		if s.unregister != nil {
			s.unregister()
		}
		// 唤醒可能仍在等待结果的调用方
		s.resultMu.Lock()
		ch := s.resultCh
		s.resultMu.Unlock()
		if ch != nil {
			select {
			case ch <- "":
			default:
			}
		}
		// closedCh 只关闭一次（Close 与 Run 返回都会尝试）
		s.closeRunCh()
		LoggerGeneral.SubInfo("BrowserClient", "OpenWebViewSession", "会话已结束 id=%s", s.ID)
	})
}

func (s *WebViewSession) closeRunCh() {
	defer func() { _ = recover() }() // closedCh 可能已被 Close 关闭
	close(s.closedCh)
}

// Close 请求关闭会话窗口（任意 goroutine 可调用；Terminate 经 Dispatch 投递到 UI 线程）
func (s *WebViewSession) Close() {
	if !s.IsAlive() {
		return
	}
	s.wv.Dispatch(func() {
		s.wv.Terminate()
	})
}

// IsAlive 会话窗口是否仍在运行
func (s *WebViewSession) IsAlive() bool {
	return atomic.LoadInt32(&s.aliveFlag) == 1
}

// ClosedCh 会话结束时关闭的通道（供外部等待）
func (s *WebViewSession) ClosedCh() <-chan struct{} {
	return s.closedCh
}

// Navigate 导航到指定 URL（经 Dispatch 投递到 UI 线程）
func (s *WebViewSession) Navigate(url string) error {
	if !s.IsAlive() {
		return errors.New("会话已关闭")
	}
	s.navigateMu.Lock()
	defer s.navigateMu.Unlock()
	s.wv.Dispatch(func() {
		if s.IsAlive() {
			s.wv.Navigate(url)
		}
	})
	return nil
}

// EvalJSON 在页面上下文执行脚本并取回结果。
// script 内使用 return 语句返回值，返回值会被 JSON.stringify 后回传（应为对象）。
func (s *WebViewSession) EvalJSON(script string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if !s.IsAlive() {
		return "", errors.New("会话已关闭")
	}
	s.navigateMu.Lock()
	defer s.navigateMu.Unlock()
	if !s.IsAlive() {
		return "", errors.New("会话已关闭")
	}

	ch := make(chan string, 1)
	s.resultMu.Lock()
	s.resultCh = ch
	s.resultMu.Unlock()

	wrapped := `(function(){var __r;try{__r=JSON.stringify((function(){` + script + `})());}catch(e){__r=JSON.stringify({__eval_error:String((e&&e.message)||e)});}if(__r===undefined){__r="null";}window.__lunarReport(__r);})()`
	s.wv.Dispatch(func() {
		if s.IsAlive() {
			s.wv.Eval(wrapped)
		}
	})

	select {
	case r := <-ch:
		if r == "" {
			return "", errors.New("会话已关闭")
		}
		return r, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("Eval 超时（%s）", timeout)
	case <-s.closedCh:
		return "", errors.New("会话已关闭")
	}
}

// WaitReady 等待页面导航提交并就绪（interactive/complete）。
// prevURL 为导航前页面的 URL（用于识别跨页提交；为空则跳过提交检测只等就绪）。
func (s *WebViewSession) WaitReady(prevURL string, timeout time.Duration) (PageState, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error

	// 阶段一：等待导航提交（URL 变化或出现 loading），同 URL 重复导航时 readyState 也会经过 loading
	if prevURL != "" {
		for time.Now().Before(deadline) {
			st, err := s.snapshot()
			if err == nil && (st.URL != prevURL || st.ReadyState == "loading") {
				break
			}
			if err != nil {
				lastErr = err
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// 阶段二：等待就绪
	for {
		st, err := s.snapshot()
		if err == nil && (st.ReadyState == "interactive" || st.ReadyState == "complete") {
			return st, nil
		}
		if err != nil {
			lastErr = err
		}
		if !time.Now().Before(deadline) {
			return st, fmt.Errorf("等待页面就绪超时: %v", lastErr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Snapshot 读取当前页面状态
func (s *WebViewSession) Snapshot() (PageState, error) {
	return s.snapshot()
}

func (s *WebViewSession) snapshot() (PageState, error) {
	raw, err := s.EvalJSON(`return {rs: document.readyState, url: location.href, title: document.title};`, 3*time.Second)
	if err != nil {
		return PageState{}, err
	}
	var st PageState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return PageState{}, fmt.Errorf("解析页面状态失败: %w", err)
	}
	return st, nil
}

// GetWebViewSession 按 ID 查找会话
func GetWebViewSession(id string) (*WebViewSession, bool) {
	sessionRegistryMu.Lock()
	defer sessionRegistryMu.Unlock()
	s, ok := sessionRegistry[id]
	return s, ok
}

// LatestWebViewSession 返回最近创建的存活会话
func LatestWebViewSession() (*WebViewSession, bool) {
	sessionRegistryMu.Lock()
	defer sessionRegistryMu.Unlock()
	s, ok := sessionRegistry[sessionRegistryNew]
	if !ok || !s.IsAlive() {
		return nil, false
	}
	return s, true
}

// CloseWebViewSession 按 ID 关闭会话
func CloseWebViewSession(id string) error {
	sessionRegistryMu.Lock()
	s, ok := sessionRegistry[id]
	sessionRegistryMu.Unlock()
	if !ok {
		return errors.New("会话不存在: " + id)
	}
	s.Close()
	return nil
}
