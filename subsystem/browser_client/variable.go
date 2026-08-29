package BrowserClient

import (
	"sync"

	webview "github.com/webview/webview_go"
)

var (
	webviewMutex       sync.Mutex
	webviewInstance    webview.WebView
	webviewRunning     bool                     // 标记是否有实例在运行
	webviewClosedCh    = make(chan struct{}, 1) // 外部关闭通知
	webviewCleanupDone = make(chan struct{}, 1) // 清理完成信号，用于同步重开
)

// ==== Win32 常量（窗口图标与标题栏样式，仅 Windows 平台使用） ====
const (
	// 窗口消息与图标类型
	wmSetIcon = 0x0080 // WM_SETICON
	iconSmall = 0      // ICON_SMALL 标题栏/Alt-Tab 小图标
	iconBig   = 1      // ICON_BIG 任务栏大图标
	imgIcon   = 1      // IMAGE_ICON

	// LoadImageW 加载标志
	lrLoadFromFile = 0x00000010 // LR_LOADFROMFILE 从文件加载
	lrDefaultSize  = 0x00000040 // LR_DEFAULTSIZE 使用系统默认图标尺寸

	// DWM 窗口属性（Win11 新增标题栏着色属性）
	dwmwaBorderColor          = 34 // DWMWA_BORDER_COLOR 窗口边框色
	dwmwaCaptionColor         = 35 // DWMWA_CAPTION_COLOR 标题栏背景色
	dwmwaUseImmersiveDarkMode = 20 // DWMWA_USE_IMMERSIVE_DARK_MODE 深色标题栏
)
