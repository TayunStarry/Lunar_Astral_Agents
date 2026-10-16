package server

import (
	"net/http"

	"LunarSubsystem/LoggerGeneral"
)

// 桌面前端静态挂载路径
const (
	// DesktopPrefix 琉璃桌面的挂载前缀（月华消息终端占用根路径 /）
	DesktopPrefix = "/desktop/"
)

// noStoreHandler 为静态文件响应补齐缓存控制头。
//
// 背景：两个桌面前端（月华消息终端与琉璃桌面）都经 http.FileServer 提供内嵌资源，
// 而内嵌文件的 ModTime 恒为零值、且不带 ETag。浏览器据此可长期复用旧副本，
// 于是"后端已换新前端、浏览器仍在跑旧 JS"——表现为新加的日志与逻辑全都不生效
// （例如 LTPX 注入链路静默不执行，终端日志里看不到任何前端发起的新请求）。
// 前端资源体积很小，这里统一禁止缓存，保证刷新即取到与二进制一致的版本。
func noStoreHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		h.ServeHTTP(w, r)
	})
}

// registerStatic 挂载两个桌面前端的静态资源
func registerStatic(mux *http.ServeMux, roots StaticRoots) {
	// ==== 月华消息终端：根路径 ====
	// noStoreHandler 必须在 FileServer 之前设置响应头；http.ServeContent 不会覆盖已设置的
	// Cache-Control，因此"禁止缓存"对 HTML / JS / CSS 全部生效。
	if roots.Lunar != nil {
		mux.Handle("/", noStoreHandler(http.FileServer(roots.Lunar)))
	} else {
		LoggerGeneral.Warn("CorePersonality", "月华前端静态资源未就绪，根路径暂不可访问")
	}

	// ==== 琉璃桌面：/desktop/ ====
	if roots.Crystal != nil {
		// 子树挂载：/desktop/xxx → 琉璃前端内的 xxx
		mux.Handle(DesktopPrefix, noStoreHandler(http.StripPrefix(DesktopPrefix, http.FileServer(roots.Crystal))))
		// 子树根不带斜杠时显式重定向，避免被根路径的月华静态吞掉
		mux.HandleFunc("/desktop", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, DesktopPrefix, http.StatusMovedPermanently)
		})
		LoggerGeneral.Info("CorePersonality", "琉璃桌面已挂载于 %s（静态资源禁止缓存）", DesktopPrefix)
	} else {
		LoggerGeneral.Warn("CorePersonality", "琉璃前端静态资源未就绪，%s 暂不可访问", DesktopPrefix)
	}
}
