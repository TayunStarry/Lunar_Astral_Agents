//go:build windows

package BrowserClient

import (
	"LunarSubsystem/GeneralConfig"
	"LunarSubsystem/LoggerGeneral"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	webview "github.com/webview/webview_go"
)

// Windows 平台下的 user32 / dwmapi 动态库与函数句柄
var (
	winUser32 = syscall.NewLazyDLL("user32.dll")
	winDwmapi = syscall.NewLazyDLL("dwmapi.dll")

	procLoadImageW            = winUser32.NewProc("LoadImageW")
	procSendMessageW          = winUser32.NewProc("SendMessageW")
	procDwmSetWindowAttribute = winDwmapi.NewProc("DwmSetWindowAttribute")
)

// applyWindowStyle 应用窗口图标与标题栏样式（需在主线程调用）
func applyWindowStyle(w webview.WebView) {
	hwnd := uintptr(w.Window())
	if hwnd == 0 {
		LoggerGeneral.SubError("BrowserClient", "applyWindowStyle", "获取窗口句柄失败，跳过样式设置")
		return
	}

	if *GeneralConfig.WebViewIconPath != "" {
		setWindowIcon(hwnd, *GeneralConfig.WebViewIconPath)
	}

	if *GeneralConfig.WebViewCaptionColor != "" ||
		*GeneralConfig.WebViewBorderColor != "" ||
		*GeneralConfig.WebViewDarkTitleBar {
		setTitleBarStyle(hwnd)
	}
}

// setWindowIcon 通过 WM_SETICON 设置窗口小图标（标题栏/Alt-Tab）与大图标（任务栏）
func setWindowIcon(hwnd uintptr, iconPath string) {
	resolved := resolveIconPath(iconPath)
	pathPtr, err := syscall.UTF16PtrFromString(resolved)
	if err != nil {
		LoggerGeneral.SubError("BrowserClient", "setWindowIcon", "图标路径转宽字符失败: %v", err)
		return
	}

	// LoadImageW(nil, path, IMAGE_ICON, 0, 0, LR_LOADFROMFILE|LR_DEFAULTSIZE)
	hIcon, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(imgIcon),
		0,
		0,
		uintptr(lrLoadFromFile|lrDefaultSize),
	)
	if hIcon == 0 {
		LoggerGeneral.SubError("BrowserClient", "setWindowIcon", "加载图标失败: %s (实际尝试路径: %s)", iconPath, resolved)
		return
	}

	procSendMessageW.Call(hwnd, uintptr(wmSetIcon), uintptr(iconSmall), hIcon)
	procSendMessageW.Call(hwnd, uintptr(wmSetIcon), uintptr(iconBig), hIcon)
	LoggerGeneral.SubInfo("BrowserClient", "setWindowIcon", "窗口图标已设置: %s", resolved)
}

// resolveIconPath 将图标路径解析为实际可用的路径：
// 绝对路径直接使用；相对路径依次尝试「可执行文件所在目录」「当前工作目录」
// 两个位置都不存在时返回第一个候选路径，便于错误日志展示
func resolveIconPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	var candidates []string
	if exePath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), path))
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, path))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return path
}

// setTitleBarStyle 通过 DWM 设置标题栏背景色、边框色与深色模式
func setTitleBarStyle(hwnd uintptr) {
	if color, ok := parseHexColor(*GeneralConfig.WebViewCaptionColor); ok {
		procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaCaptionColor),
			uintptr(unsafe.Pointer(&color)), unsafe.Sizeof(color))
	}
	if color, ok := parseHexColor(*GeneralConfig.WebViewBorderColor); ok {
		procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaBorderColor),
			uintptr(unsafe.Pointer(&color)), unsafe.Sizeof(color))
	}
	if *GeneralConfig.WebViewDarkTitleBar {
		dark := uint32(1)
		procDwmSetWindowAttribute.Call(hwnd, uintptr(dwmwaUseImmersiveDarkMode),
			uintptr(unsafe.Pointer(&dark)), unsafe.Sizeof(dark))
	}
	LoggerGeneral.SubInfo("BrowserClient", "setTitleBarStyle", "标题栏样式已应用")
}

// parseHexColor 解析 #RRGGBB / 0xRRGGBB / RRGGBB 为 COLORREF（0x00BBGGRR）
func parseHexColor(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	r := uint32(v >> 16 & 0xFF)
	g := uint32(v >> 8 & 0xFF)
	b := uint32(v & 0xFF)
	return (b << 16) | (g << 8) | r, true
}
