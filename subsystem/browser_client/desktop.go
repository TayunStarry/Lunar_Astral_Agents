package BrowserClient

// 桌面窗口组：单进程托管多个常驻桌面窗口（月华消息终端 + 琉璃桌面）。
//
// 与单实例主窗口（StartWebViewBrowser）不同，本组支持多个窗口并存：
// 每个窗口在独立 OS 线程上创建并运行自己的 webview 实例。
//
// 退出判定注意：**不能只依赖 `Run()` 返回**。实测 webview_go 在 Windows/WebView2 下，
// 窗口被用户关闭后其 `Run()` 可能不返回（窗口与渲染进程确已销毁，事件循环却不退出），
// 且两个窗口行为还不一致（一个会返回、一个不会）。若只按 Run() 计数「全部关闭」，
// 残留的那个窗口会使计数永不归零，表现为「窗口全关而服务端不退」。
//
// 因此：本组仅在**确实全部窗口都从注册表移除**（即所有 Run() 都返回）时发出信号，
// 作为快速路径；窗口是否「全部关闭」不再作为进程退出依据——控制台（TUI）是程序主界面，
// 窗口全关后仍可在控制台「前端页面 管理」中重新唤起（见 core_personality/app/OpenFrontendWindows）。
// 发出信号后由 CloseAllDesktopWindows 对残留窗口 Terminate，交由控制台退出时的
// GracefulShutdown 兜底超时保证进程一定退出。

import (
	"errors"
	"fmt"
	"runtime"
	"sync"

	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"

	webview "github.com/webview/webview_go"
)

// DesktopWindowSpec 桌面窗口规格
type DesktopWindowSpec struct {
	// ID 窗口标识（用于聚焦与去重，如 lunar / crystal）
	ID string
	// URL 初始地址
	URL string
	// Title 窗口标题
	Title string
	// Width/Height 窗口尺寸
	Width  int
	Height int
	// IconPath 窗口图标（.ico 路径，空则沿用默认图标）
	IconPath string
	// InitJS 页面注入脚本（每个新文档创建时执行）
	InitJS string
}

// desktopWindow 已打开的桌面窗口
type desktopWindow struct {
	spec DesktopWindowSpec
	wv   webview.WebView
}

var (
	// desktopMu 保护桌面窗口注册表
	desktopMu sync.Mutex
	// desktopWindows 已打开的桌面窗口（ID → 窗口）
	desktopWindows = map[string]*desktopWindow{}
	// desktopOpenedTotal 成功打开的窗口累计数（用于判断「全部关闭」）
	desktopOpenedTotal int
	// desktopClosedCh 全部桌面窗口关闭信号
	desktopClosedCh = make(chan struct{}, 1)
	// desktopCloseOnce 保证关闭信号只发一次
	desktopCloseOnce sync.Once
)

// BuildReturnButtonJS 生成「返回主页面」悬浮按钮的页面注入脚本。
// 供桌面窗口在任意页面（含外部链接跳转后）提供回到主界面的入口。
func BuildReturnButtonJS(mainURL string) string {
	return buildReturnButtonJS(mainURL)
}

// OpenDesktopWindow 打开一个桌面窗口；同 ID 窗口已存在时改为聚焦
func OpenDesktopWindow(spec DesktopWindowSpec) error {
	if spec.ID == "" {
		return errors.New("桌面窗口缺少 ID")
	}
	if spec.Width <= 0 {
		spec.Width = 1080
	}
	if spec.Height <= 0 {
		spec.Height = 810
	}

	desktopMu.Lock()
	if _, exists := desktopWindows[spec.ID]; exists {
		desktopMu.Unlock()
		if err := FocusDesktopWindow(spec.ID); err == nil {
			return nil
		}
		// 已登记但句柄失效：webview 的 Run() 在窗口被关闭后可能不返回
		// （见文件头说明），残留条目会使唤起永远聚焦到死窗口而无任何可见效果。
		// 清除残留后走下方重建流程。
		desktopMu.Lock()
		delete(desktopWindows, spec.ID)
		desktopMu.Unlock()
	}
	win := &desktopWindow{spec: spec}
	desktopWindows[spec.ID] = win
	desktopMu.Unlock()

	created := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		w := webview.New(*GeneralConfig.Developer)
		if w == nil {
			desktopMu.Lock()
			delete(desktopWindows, spec.ID)
			desktopMu.Unlock()
			created <- fmt.Errorf("创建桌面窗口 [%s] 失败", spec.ID)
			return
		}

		desktopMu.Lock()
		win.wv = w
		desktopMu.Unlock()

		w.SetTitle(spec.Title)
		w.SetSize(spec.Width, spec.Height, webview.HintNone)
		if *GeneralConfig.WebViewMinWidth > 0 && *GeneralConfig.WebViewMinHeight > 0 {
			w.SetSize(*GeneralConfig.WebViewMinWidth, *GeneralConfig.WebViewMinHeight, webview.HintMin)
		}
		applyDesktopWindowStyle(w, spec.IconPath)
		if spec.InitJS != "" {
			w.Init(spec.InitJS)
		}
		if spec.URL != "" {
			w.Navigate(spec.URL)
		}

		desktopMu.Lock()
		desktopOpenedTotal++
		desktopMu.Unlock()
		created <- nil

		// 阻塞至窗口关闭
		w.Run()

		// 先登记关闭并发出整体关闭信号，再释放底层资源：
		// Destroy 可能长时间阻塞，置于信号之前会拖住退出。
		desktopMu.Lock()
		delete(desktopWindows, spec.ID)
		remaining := len(desktopWindows)
		desktopMu.Unlock()

		LoggerGeneral.SubInfo("BrowserClient", "OpenDesktopWindow", "桌面窗口已关闭 id=%s，剩余 %d", spec.ID, remaining)

		// 快速路径：全部窗口的 Run() 均已返回（可靠判据见宿主侧前端会话监视）
		if remaining == 0 {
			desktopCloseOnce.Do(func() {
				select {
				case desktopClosedCh <- struct{}{}:
				default:
				}
			})
		}

		w.Destroy()
	}()

	if err := <-created; err != nil {
		return err
	}

	LoggerGeneral.SubInfo("BrowserClient", "OpenDesktopWindow", "桌面窗口已打开 id=%s url=%s", spec.ID, spec.URL)
	return nil
}

// FocusDesktopWindow 将指定桌面窗口恢复并置前；句柄已失效时返回错误
func FocusDesktopWindow(id string) error {
	desktopMu.Lock()
	win, exists := desktopWindows[id]
	var wv webview.WebView
	if exists {
		wv = win.wv
	}
	desktopMu.Unlock()

	if !exists || wv == nil {
		return fmt.Errorf("桌面窗口 [%s] 不存在", id)
	}
	if !focusDesktopWindow(wv) {
		return fmt.Errorf("桌面窗口 [%s] 句柄已失效", id)
	}
	return nil
}

// RestartDesktopWindow 重启桌面窗口（「重启前端」与前端守护唤起专用）：
//   - 窗口存活：经 Dispatch 在窗口 UI 线程重载页面（重启语义要求页面重新加载，
//     而非仅聚焦），随后还原置前；
//   - 窗口已死（Run 未返回的残留条目）：清理登记后重建；
//   - 未打开：直接创建。
func RestartDesktopWindow(spec DesktopWindowSpec) error {
	if spec.ID == "" {
		return errors.New("桌面窗口缺少 ID")
	}

	desktopMu.Lock()
	win, exists := desktopWindows[spec.ID]
	var wv webview.WebView
	if exists {
		wv = win.wv
	}
	desktopMu.Unlock()

	if exists && wv != nil && desktopWindowAlive(wv) {
		if spec.URL != "" {
			wv.Dispatch(func() { wv.Navigate(spec.URL) })
		}
		focusDesktopWindow(wv)
		return nil
	}
	if exists {
		// 句柄已失效：清理残留后走重建
		desktopMu.Lock()
		delete(desktopWindows, spec.ID)
		desktopMu.Unlock()
	}
	return OpenDesktopWindow(spec)
}

// CloseDesktopWindow 关闭指定桌面窗口
func CloseDesktopWindow(id string) error {
	desktopMu.Lock()
	win, exists := desktopWindows[id]
	var wv webview.WebView
	if exists {
		wv = win.wv
	}
	desktopMu.Unlock()

	if !exists {
		return fmt.Errorf("桌面窗口 [%s] 不存在", id)
	}
	if wv != nil {
		wv.Terminate()
	}
	return nil
}

// CloseAllDesktopWindows 关闭全部桌面窗口（进程退出前调用）
func CloseAllDesktopWindows() {
	desktopMu.Lock()
	windows := make([]webview.WebView, 0, len(desktopWindows))
	for _, win := range desktopWindows {
		if win.wv != nil {
			windows = append(windows, win.wv)
		}
	}
	desktopMu.Unlock()

	for _, wv := range windows {
		wv.Terminate()
	}
}

// DesktopWindowsClosed 返回「全部桌面窗口的 Run() 均已返回」的信号通道（快速路径；
// 常规退出依据宿主侧前端会话监视，见文件头说明）
func DesktopWindowsClosed() <-chan struct{} {
	return desktopClosedCh
}

// DesktopWindowIDs 返回当前已打开桌面窗口的 ID 列表（供 /window/focus 校验）
func DesktopWindowIDs() []string {
	desktopMu.Lock()
	defer desktopMu.Unlock()
	ids := make([]string, 0, len(desktopWindows))
	for id := range desktopWindows {
		ids = append(ids, id)
	}
	return ids
}

// HasOpenedDesktopWindow 是否曾成功打开过桌面窗口
func HasOpenedDesktopWindow() bool {
	desktopMu.Lock()
	defer desktopMu.Unlock()
	return desktopOpenedTotal > 0
}
