package clients

import (
	"net/http"
	"testing"
)

// 三个前端的入口页与关键资源必须都能在内嵌树中解析出来。
//
// 三个前端现在各自独占 clients/ 下的子树（crystal/、lunar/，其中 mobile/ 是
// 月华移动端），入口页取不到就等于整个前端白屏——而 go:embed 的模式写成
// 目录名时丢失文件是编译期静默的，只有真去 Open 一次才会暴露。
func TestFrontendEntrypointsAreEmbedded(t *testing.T) {
	cases := []struct {
		name string
		fs   http.FileSystem
		path string
	}{
		{"月华消息终端入口", LunarFS(), "/index.html"},
		{"月华消息终端脚本", LunarFS(), "/js/main.js"},
		{"月华消息终端样式", LunarFS(), "/css/layout.css"},
		{"月华移动端入口", LunarFS(), "/mobile/index.html"},
		{"琉璃桌面入口", CrystalFS(), "/index.html"},
		{"琉璃桌面脚本", CrystalFS(), "/script.js"},
		{"琉璃蓝图子页", CrystalFS(), "/planning/index.html"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := tc.fs.Open(tc.path)
			if err != nil {
				t.Fatalf("打开 %s 失败（检查 clients.go 的 go:embed 模式与子目录名）: %v", tc.path, err)
			}
			defer f.Close()

			st, err := f.Stat()
			if err != nil {
				t.Fatalf("读取 %s 的元信息失败: %v", tc.path, err)
			}
			if st.IsDir() {
				t.Fatalf("%s 解析成了目录", tc.path)
			}
			if st.Size() == 0 {
				t.Fatalf("%s 内容为空", tc.path)
			}
		})
	}
}

// 两套前端必须是彼此不相交的子树。
//
// 这条断言守住"三个前端各自收纳、互不串味"的结构约束：先是两者都在同一个
// assets/ 目录下、靠文件名前缀区分，任何一个前缀写错都会静默取到另一套前端
// 的资源（页面能打开、样式与脚本却是别人的），比 404 难排查得多。
func TestFrontendTreesAreDisjoint(t *testing.T) {
	if f, err := LunarFS().Open("/planning/index.html"); err == nil {
		f.Close()
		t.Fatal("月华消息终端不应包含琉璃蓝图的 planning/ 子树")
	}
	if f, err := CrystalFS().Open("/mobile/index.html"); err == nil {
		f.Close()
		t.Fatal("琉璃桌面不应包含月华移动端的 mobile/ 子树")
	}
	if f, err := CrystalFS().Open("/js/main.js"); err == nil {
		f.Close()
		t.Fatal("琉璃桌面不应包含月华消息终端的 js/ 子树")
	}
}
