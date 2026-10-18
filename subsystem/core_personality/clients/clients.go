// Package clients 收纳三个前端的静态资源，并统一负责它们的内嵌与 http.FileSystem 投影。
//
// 三个前端是同一类东西（可被挂载的静态站点），因此集中在 clients/ 一处收纳，
// 不再散落在各自人格包里：
//
//	clients/lunar/    月华消息终端（挂载于 /，其 mobile/ 子树即月华移动端）
//	clients/crystal/  琉璃桌面（挂载于 /desktop/，含 file-runner/ 编辑运行等内置子页）
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

// EmbeddedFiles 内嵌的三套前端资源（crystal/ 与 lunar/ 各自一棵子树）
//
// 注意：go:embed 的模式必须与下方 subFS 的子树名逐字一致。
// 历史缺陷正是两边不一致——指令写 `web/*`，读取方却按 "assets/xxx" 取，
// go 编译器与 tsc 都不报错，只有真跑一遍 handler 才暴露 500。
//
//go:embed crystal/* lunar/*
var EmbeddedFiles embed.FS

// CrystalFS 琉璃桌面的静态文件系统（crystal/ 子树）。
// 由装配层经 server.StaticRoots 挂载于 /desktop/。
func CrystalFS() http.FileSystem {
	return subFS("crystal")
}

// LunarFS 月华消息终端的静态文件系统（lunar/ 子树，其 mobile/ 即月华移动端）。
// 由装配层经 server.StaticRoots 挂载于根路径 /。
func LunarFS() http.FileSystem {
	return subFS("lunar")
}

// subFS 把内嵌文件系统投影为某个前端子树的根。
//
// 子树名写错时 fs.Sub 会返回错误而不是空树；此处直接 panic —— 子树名与 go:embed 模式同处一个文件，一旦改漏应当在启动瞬间炸掉，而不是把三个前端
// 悄悄变成一片 404。
func subFS(dir string) http.FileSystem {
	sub, err := fs.Sub(EmbeddedFiles, dir)
	if err != nil {
		panic("clients: 内嵌前端子树 [" + dir + "] 不存在（检查 clients.go 的 go:embed 模式）: " + err.Error())
	}
	return http.FS(sub)
}
