package server

// static.go 前端静态资源挂载（消息终端 / 移动端 / 琉璃桌面 / 资源库 / 共享依赖）。

import (
	"net/http"
	"strings"

	"LunarSubsystem/LunarHarness/engine/console/logger"
)

// 前端静态挂载路径
const (
	// DesktopPrefix 琉璃桌面的挂载前缀（月华消息终端占用根路径 /）
	DesktopPrefix = "/desktop/"
	// MobilePrefix 月华移动端的挂载前缀
	MobilePrefix = "/mobile/"
	// LibraryPrefix 资源库的挂载前缀（只手动打开，不随程序自动弹出）
	LibraryPrefix = "/library/"
	// ReliancePrefix 前端共享依赖的挂载前缀（字体图标 / 高亮 / 公式 / 音乐库 / 渲染引擎）
	ReliancePrefix = "/reliance/"
	// SharedPrefix 桌面端与移动端共用 ES Module 共享层的挂载前缀
	SharedPrefix = "/shared/"
)

// noStoreHandler 为静态文件响应补齐缓存控制头。
//
// 背景：各前端都经 http.FileServer 提供内嵌资源，而内嵌文件的 ModTime 恒为零值、
// 且不带 ETag。浏览器据此可长期复用旧副本，于是"后端已换新前端、浏览器仍在跑旧 JS"。
// 前端资源体积很小，这里统一禁止缓存，保证刷新即取到与二进制一致的版本。
func noStoreHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		h.ServeHTTP(w, r)
	})
}

// registerStatic 挂载全部前端静态资源。
func registerStatic(mux *http.ServeMux, roots StaticRoots) {
	// ==== 月华消息终端：根路径 ====
	// noStoreHandler 必须在 FileServer 之前设置响应头；http.ServeContent 不会覆盖已设置的
	// Cache-Control，因此"禁止缓存"对 HTML / JS / CSS 全部生效。
	if roots.Lunar != nil {
		mux.Handle("/", noStoreHandler(http.FileServer(roots.Lunar)))
	} else {
		logger.Warn("LunarHarness", "月华前端静态资源未就绪，根路径暂不可访问")
	}

	mountSubtree(mux, MobilePrefix, roots.Mobile, "月华移动端")
	mountSubtree(mux, DesktopPrefix, roots.Crystal, "琉璃桌面")
	mountSubtree(mux, LibraryPrefix, roots.Library, "资源库")
	mountSubtree(mux, ReliancePrefix, roots.Reliance, "前端共享依赖")
	mountSubtree(mux, SharedPrefix, roots.Shared, "前端共享模块")
}

// mountSubtree 以子树方式挂载一个前端，并为不带斜杠的前缀补一次重定向。
func mountSubtree(mux *http.ServeMux, prefix string, files http.FileSystem, label string) {
	if files == nil {
		logger.Warn("LunarHarness", "%s静态资源未就绪，%s 暂不可访问", label, prefix)
		return
	}
	mux.Handle(prefix, noStoreHandler(http.StripPrefix(prefix, http.FileServer(files))))
	base := strings.TrimSuffix(prefix, "/")
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, prefix, http.StatusMovedPermanently)
	})
	logger.Info("LunarHarness", "%s已挂载于 %s（静态资源禁止缓存）", label, prefix)
}
