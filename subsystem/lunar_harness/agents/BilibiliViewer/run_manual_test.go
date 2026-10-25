package BilibiliViewer

// 手动端到端：真实浏览器 + 登录扫码 + 选片/下载/理解/评论/动态/回复。
//   $env:BILI_LIFE_RUN="1"; go test -run TestManualRun -v -timeout 20m ./agents/BilibiliViewer
// 默认跳过，避免普通测试打开浏览器或调用模型。

import (
	"os"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestManualRun(t *testing.T) {
	if os.Getenv("BILI_LIFE_RUN") != "1" {
		t.Skip("设置 BILI_LIFE_RUN=1 才执行真实 B 站生活")
	}
	// 测试二进制不在 exe 目录，必须显式指向仓库的 local_data，否则切片/临时文件会落到临时目录。
	config.LocalDir = `D:\Lunar_Astral_Agents\local_data`
	// 手动测试给扫码留 3 分钟；生产仍是 60 秒。
	testLoginWait = 3 * time.Minute
	if err := Run(); err != nil {
		t.Fatalf("B站生活失败: %v", err)
	}
}
