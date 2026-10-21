package BilibiliLife

// 手动冒烟：只打开 B 站首页并读一次登录态，不扫码、不刷视频。
// 默认跳过；需要真窗口时：
//   $env:BILI_LIFE_PROBE="1"; go test -run TestManualBrowserLoginProbe -v ./agents/BilibiliLife

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestManualBrowserLoginProbe(t *testing.T) {
	if os.Getenv("BILI_LIFE_PROBE") != "1" {
		t.Skip("设置 BILI_LIFE_PROBE=1 才打开真实浏览器")
	}
	browser, err := openBilibiliBrowser()
	if err != nil {
		t.Fatalf("打开浏览器失败: %v", err)
	}
	defer browser.close()
	time.Sleep(2 * time.Second)
	state, err := browser.loginState()
	if err != nil {
		t.Fatalf("读取登录态失败: %v", err)
	}
	t.Logf("B站登录态: loggedIn=%v hasLogin=%v url=%s title=%s", state.LoggedIn, state.HasLogin, state.URL, state.Title)
	if !state.LoggedIn && !state.HasLogin {
		t.Log("页面既没有头像也没有登录按钮：选择器可能需要按真实 DOM 调整")
	}
	if !state.LoggedIn && state.HasLogin {
		if err := browser.clickLogin(); err != nil {
			t.Fatalf("点击登录失败: %v", err)
		}
		time.Sleep(2 * time.Second)
		after, _ := browser.loginState()
		t.Logf("点击登录后: loggedIn=%v hasLogin=%v url=%s title=%s", after.LoggedIn, after.HasLogin, after.URL, after.Title)
		if !after.LoggedIn && !strings.Contains(after.URL, "passport.bilibili.com") {
			if err := browser.navigate(biliLoginURL, 25*time.Second); err != nil {
				t.Fatalf("打开 passport 登录页失败: %v", err)
			}
			time.Sleep(3 * time.Second)
			page, _ := browser.loginState()
			t.Logf("passport 登录页: url=%s title=%s", page.URL, page.Title)
		}
	}
}
