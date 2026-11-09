package runtime

// video_test.go video.frames 采样计划：times > count > fps 的优先级与帧数上限。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/proxy"
	multimodal "LunarSubsystem/LunarHarness/engine/multimodal"
	"LunarSubsystem/LunarHarness/engine/netsource"
)

func TestVideoSamplingPlanPriority(t *testing.T) {
	// times 显式时间点优先
	plan := videoSamplingPlan(10, map[string]any{"times": []any{int64(1), int64(4)}})
	if len(plan) != 2 || plan[0] != 1 || plan[1] != 4 {
		t.Fatalf("times 应精确抽取，实得 %+v", plan)
	}

	// count 等距
	plan = videoSamplingPlan(10, map[string]any{"count": int64(3)})
	if len(plan) != 3 {
		t.Fatalf("count 应等距抽取 3 帧，实得 %+v", plan)
	}

	// fps 均匀网格（10 秒 × 2fps = 21 点）
	plan = videoSamplingPlan(10, map[string]any{"fps": float64(2)})
	if len(plan) != 21 {
		t.Fatalf("fps 抽帧数不正确，实得 %d", len(plan))
	}
}

func TestVideoSamplingPlanCap(t *testing.T) {
	// fps 过高时按上限等距抽样，不会超过 MaxVideoFrames
	plan := videoSamplingPlan(120, map[string]any{"fps": float64(60)})
	if len(plan) > multimodal.MaxVideoFrames {
		t.Fatalf("抽帧数不应超过上限 %d，实得 %d", multimodal.MaxVideoFrames, len(plan))
	}
}

// TestVideoSourceToLocalLANServiceURL 手机端 / 扩展包以局域网 HTTPS 地址传入视频时，
// 必须直接复用本机 local_data 文件，不下载、不产生临时清理动作。
func TestVideoSourceToLocalLANServiceURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/lan-sample.mp4"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	lan := ""
	for _, ip := range proxy.LocalIPs() {
		if ip != "127.0.0.1" {
			lan = ip
			break
		}
	}
	if lan == "" {
		t.Skip("本机没有可用的局域网地址")
	}

	source := "https://" + lan + ":36979/file/read/" + rel
	local, cleanup, err := videoSourceToLocal(nil, source)
	if err != nil {
		t.Fatalf("局域网视频地址应直接本地读盘: %v", err)
	}
	if cleanup != nil {
		t.Fatal("本地文件不得按临时下载件清理")
	}
	if local != abs {
		t.Fatalf("本地路径不正确: 期望 %s，实得 %s", abs, local)
	}
}

// TestVideoRemoteURLKeepsTLSVerification 非本机地址仍走网络；
// 默认配置下证书校验保持开启，只有显式开启 server.insecure_tls 才放开。
func TestVideoRemoteURLKeepsTLSVerification(t *testing.T) {
	oldInsecure := config.InsecureTLS
	config.InsecureTLS = false
	t.Cleanup(func() { config.InsecureTLS = oldInsecure })

	const remote = "https://example.com/videos/sample.mp4"
	if _, ok := netsource.LocalPath(remote); ok {
		t.Fatal("非本机地址不应被解析为本地文件")
	}
	if netsource.ShouldSkipVerify(remote) {
		t.Fatal("默认配置下远程地址不得跳过证书校验")
	}

	// 校验开启的客户端必须拒绝自签证书，证明豁免没有扩散到远程地址
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("video"))
	}))
	defer srv.Close()
	if _, err := netsource.NewHTTPClient(2*time.Second, false).Get(srv.URL); err == nil {
		t.Fatal("证书校验开启时不应信任自签测试证书")
	}

	config.InsecureTLS = true
	if !netsource.ShouldSkipVerify(remote) {
		t.Fatal("开启 server.insecure_tls 后远程地址应允许跳过证书校验")
	}
}
