package multimodal

// audio_test.go 音频本地化回归：局域网 HTTPS 服务地址走本地读盘，
// 远程地址仍走网络且默认保留证书校验。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/proxy"
	"LunarSubsystem/LunarHarness/engine/netsource"
)

// TestLocalizeAudioInputLANServiceURL 手机端以局域网 HTTPS 地址传入音频时，
// 必须直接映射到本机 local_data 文件，不发起网络请求，也不产生临时清理动作。
func TestLocalizeAudioInputLANServiceURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "audios/lan-sample.mp3"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-audio"), 0o644); err != nil {
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
	local, cleanup, err := localizeAudioInput(source)
	if err != nil {
		t.Fatalf("局域网音频地址应直接本地读盘: %v", err)
	}
	if cleanup != nil {
		t.Fatal("本地文件不得按临时下载件清理")
	}
	if local != abs {
		t.Fatalf("本地路径不正确: 期望 %s，实得 %s", abs, local)
	}
}

// TestAudioRemoteURLKeepsTLSVerification 非本机地址仍走网络；
// 默认配置下证书校验保持开启，只有显式开启 server.insecure_tls 才放开。
func TestAudioRemoteURLKeepsTLSVerification(t *testing.T) {
	oldInsecure := config.InsecureTLS
	config.InsecureTLS = false
	t.Cleanup(func() { config.InsecureTLS = oldInsecure })

	const remote = "https://example.com/audios/sample.mp3"
	if _, ok := netsource.LocalPath(remote); ok {
		t.Fatal("非本机地址不应被解析为本地文件")
	}
	if netsource.ShouldSkipVerify(remote) {
		t.Fatal("默认配置下远程地址不得跳过证书校验")
	}

	// 校验开启的客户端必须拒绝自签证书，证明豁免没有扩散到远程地址
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("audio"))
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
