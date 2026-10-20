package lunarloop

// message_test.go 图片推送地址的形态约束
//
// pushImage 把绘图/截图结果交给前端与 QQ 适配器。地址必须是 /file/read/<内容仓库路径>；
// 若把句柄（@I.00x）拼进路径，宿主会返回 404，QQ 侧表现为
// 「下载文件失败: Not Found」。这条用例钉住适配器边界上的 URL 形态。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/GeneralConfig"
)

func TestRegisterPushedImageKeepsReadableReadURL(t *testing.T) {
	_ = newBridgeTestRuntime(t)
	rel := "multimedia/generated/qq-send.png"
	abs := filepath.Join(*GeneralConfig.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("创建生成目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("fake-png-bytes"), 0644); err != nil {
		t.Fatalf("写入测试图片失败: %v", err)
	}

	got := registerPushedImage("/file/read/" + rel)
	if strings.Contains(got, "@") {
		t.Fatalf("推送地址含句柄，前端/QQ 会 404: %q", got)
	}
	if got != "/file/read/"+rel {
		t.Fatalf("推送地址应为 /file/read/<内容仓库路径>，实得 %q", got)
	}
}
