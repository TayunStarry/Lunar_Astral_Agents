package crystal

import (
	"CorePersonality/engine/ltp9"
	"io/fs"
	"net/http"
)

// Gethierarchy 返回琉璃桌面的静态文件系统（内嵌 web 目录子树）。
// 由装配层经 server.StaticRoots 挂载于 /desktop/，月华消息终端占用根路径 /。
func Gethierarchy() http.FileSystem {
	subFS, err := fs.Sub(EmbeddedFiles, "web")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}

// HandleInbound 消费 /ws 入站消息：ltp9/* 调试信封交由 LTP9 引擎处理，其余旁路。
// 由装配层注册为集线器的入站订阅者。
func HandleInbound(data []byte) {
	StarLTP.HandleInbound(data)
}

// Close 关闭琉璃人格格持有的引擎资源（进程退出前调用）
func Close() {
	StarLTP.Close()
}
