// Package clients 收纳四个前端的静态资源，并统一负责它们的内嵌与 http.FileSystem 投影。
//
// 四个前端是同一类东西（可被挂载的静态站点），因此集中在 clients/ 一处收纳：
//
//	clients/lunar/     月华消息终端（挂载于 /）
//	clients/mobile/    月华移动端（挂载于 /mobile/）
//	clients/crystal/   琉璃桌面（挂载于 /desktop/，含 file-runner/ 编辑运行等内置子页）
//	clients/library/   资源库（挂载于 /library/，手动打开、不随程序自动弹出）
//	clients/reliance/  前端共享依赖（挂载于 /reliance/，字体图标 / 高亮 / 公式 / 音乐库 / 渲染引擎）
//	clients/shared/    桌面端与移动端共用 ES Module（挂载于 /shared/，由两端按 URL 引用）
//
// reliance（依赖）里放的是**没有 metadata.json、不属于扩展包**的前端资源：
// 它们不参与 LTPX 安装与更新包分发，只随程序本体一起编译。
//
// shared（共享层）里放的是两端同构的交互逻辑（文件、WebSocket、音频队列、语音、
// Markdown 等），不包含任何一端独有的界面样式与业务规则。
//
// 归属原则：一个 go:embed 指令只能嵌入**本包目录之下**的文件，所以"资源树"与
// "内嵌它的包"必须同处一层。clients 包落在 clients/clients.go 正是为此——
// 资源在 clients/ 下，内嵌与投影代码也在 clients/ 下，两者不可能错位。
package clients

import (
	"embed"
	"io/fs"
	"net/http"
)

// EmbeddedFiles 内嵌的前端资源子树
//
// 注意：go:embed 的模式必须与下方 subFS 的子树名逐字一致。
// 历史缺陷正是两边不一致——指令写 `web/*`，读取方却按 "assets/xxx" 取，
// go 编译器与 tsc 都不报错，只有真跑一遍 handler 才暴露 500。
//
//go:embed crystal/* lunar/* mobile/* library/* reliance/* shared/*
var EmbeddedFiles embed.FS

// CrystalFS 琉璃桌面的静态文件系统（crystal/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /desktop/。
func CrystalFS() http.FileSystem {
	return subFS("crystal")
}

// LunarFS 月华消息终端的静态文件系统（lunar/ 子树）。
// 由装配层经 server.StaticRoots 挂载于根路径 /。
func LunarFS() http.FileSystem {
	return subFS("lunar")
}

// MobileFS 月华移动端的静态文件系统（mobile/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /mobile/。
func MobileFS() http.FileSystem {
	return subFS("mobile")
}

// LibraryFS 资源库的静态文件系统（library/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /library/，只能由用户手动打开。
func LibraryFS() http.FileSystem {
	return subFS("library")
}

// RelianceFS 前端共享依赖的静态文件系统（reliance/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /reliance/，供各前端与扩展包按 URL 引用。
func RelianceFS() http.FileSystem {
	return subFS("reliance")
}

// SharedFS 桌面端与移动端共用 ES Module 的静态文件系统（shared/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /shared/，两端按 URL 直接 import。
func SharedFS() http.FileSystem {
	return subFS("shared")
}

// subFS 把内嵌文件系统投影为某个前端子树的根。
//
// 子树名写错时 fs.Sub 会返回错误而不是空树；此处直接 panic —— 子树名与 go:embed 模式同处一个文件，一旦改漏应当在启动瞬间炸掉，而不是把前端
// 悄悄变成一片 404。
func subFS(dir string) http.FileSystem {
	sub, err := fs.Sub(EmbeddedFiles, dir)
	if err != nil {
		panic("clients: 内嵌前端子树 [" + dir + "] 不存在（检查 clients.go 的 go:embed 模式）: " + err.Error())
	}
	return http.FS(sub)
}
