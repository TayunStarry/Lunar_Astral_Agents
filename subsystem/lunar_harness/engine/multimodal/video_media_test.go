package multimodal

// video_media_test.go 视频分析入口（watch / keyframe）的来源本地化与清理约定。
//
// 约定：本机服务地址直接复用 LocalDir 下的原文件（cleanup 为 nil），
// 只有远程回源下载的临时文件才返回 cleanup；失败时不得返回脏路径。

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

// lanHost 取本机第一个非回环地址，没有则跳过测试
func lanHost(t *testing.T) string {
	t.Helper()
	for _, ip := range proxy.LocalIPs() {
		if ip != "127.0.0.1" {
			return ip
		}
	}
	t.Skip("本机没有可用的局域网地址")
	return ""
}

// TestLocalizeVideoSourceLANServiceURL 手机端以局域网 HTTPS 地址传入视频时，
// 必须直接返回 LocalDir 下的原文件，cleanup 为 nil，且原文件不会被删除。
func TestLocalizeVideoSourceLANServiceURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/lan-video.mp4"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	source := "https://" + lanHost(t) + ":36979/file/read/" + rel

	local, cleanup, err := localizeVideoSource(source)
	if err != nil {
		t.Fatalf("局域网视频地址应直接本地读盘: %v", err)
	}
	if cleanup != nil {
		t.Fatal("本地原文件不得返回清理函数")
	}
	if local != abs {
		t.Fatalf("本地路径不正确: 期望 %s，实得 %s", abs, local)
	}

	// 模拟调用方按约定处理：cleanup 为 nil 时不执行删除
	if cleanup != nil {
		cleanup()
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("原视频被误删: %v", err)
	}
}

// TestVideoAnalysisRemoteURLKeepsTLSVerification 非本机地址仍走网络；
// 默认配置下证书校验保持开启，只有显式开启 server.insecure_tls 才放开。
func TestVideoAnalysisRemoteURLKeepsTLSVerification(t *testing.T) {
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

// TestDownloadToTempFileCleansTempOnRemoteFailure 远程回源非 2xx 或中途断流时，
// 必须返回错误、空路径与 nil cleanup，且不残留临时文件。
func TestDownloadToTempFileCleansTempOnRemoteFailure(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "非2xx",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
			},
		},
		{
			name: "中途断流",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "2048")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("partial"))
				panic(http.ErrAbortHandler)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			local, cleanup, err := downloadToTempFile(srv.URL + "/video.mp4")
			if err == nil {
				t.Fatal("回源失败时应返回错误")
			}
			if local != "" {
				t.Fatalf("失败时不得返回路径，实得 %s", local)
			}
			if cleanup != nil {
				t.Fatal("失败时不应返回清理函数")
			}

			entries, readErr := os.ReadDir(tempDir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("失败后残留临时文件: %v", entries)
			}
		})
	}
}
