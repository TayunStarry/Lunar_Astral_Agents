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

// registerStatic 挂载两个桌面前端的静态资源
func registerStatic(mux *http.ServeMux, roots StaticRoots) {
	// ==== 月华消息终端：根路径 ====
	if roots.Lunar != nil {
		mux.Handle("/", http.FileServer(roots.Lunar))
	} else {
		LoggerGeneral.Warn("CorePersonality", "月华前端静态资源未就绪，根路径暂不可访问")
	}

	// ==== 琉璃桌面：/desktop/ ====
	if roots.Crystal != nil {
		// 子树挂载：/desktop/xxx → 琉璃前端内的 xxx
		mux.Handle(DesktopPrefix, http.StripPrefix(DesktopPrefix, http.FileServer(roots.Crystal)))
		// 子树根不带斜杠时显式重定向，避免被根路径的月华静态吞掉
		mux.HandleFunc("/desktop", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, DesktopPrefix, http.StatusMovedPermanently)
		})
		LoggerGeneral.Info("CorePersonality", "琉璃桌面已挂载于 %s", DesktopPrefix)
	} else {
		LoggerGeneral.Warn("CorePersonality", "琉璃前端静态资源未就绪，%s 暂不可访问", DesktopPrefix)
	}
}
