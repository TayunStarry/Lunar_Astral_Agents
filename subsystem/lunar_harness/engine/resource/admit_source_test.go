package resource

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
	"LunarSubsystem/LunarHarness/engine/console/proxy"
)

func TestResolveLocalSourceStripsServiceRoutePrefix(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/8929c1c1bdf053bc.jpg"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-image"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"/file/read/" + rel,
		"file/read/" + rel,
		"/file/miniature/" + rel,
		"file/miniature/" + rel,
	} {
		resolved, err := resolveLocalSource(source)
		if err != nil {
			t.Fatalf("%s 应能解析到本地文件: %v", source, err)
		}
		if resolved != abs {
			t.Fatalf("%s 解析结果错误: %s", source, resolved)
		}
	}
}

// TestResolveLocalSourceAcceptsLANServiceURL 手机端以局域网地址访问客户端时，
// `${origin}/file/read/<rel>` 必须直接映射到本地文件，不能当作远程 URL 回源自签 HTTPS。
func TestResolveLocalSourceAcceptsLANServiceURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/2a7faad02cb9b7e9.png"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-image"), 0o644); err != nil {
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
	if relPath, ok := localPathFromServiceURL(source); !ok || relPath != rel {
		t.Fatalf("局域网服务地址应解析为 %q，实际 %q（ok=%v）", rel, relPath, ok)
	}
	resolved, err := resolveLocalSource(source)
	if err != nil || resolved != abs {
		t.Fatalf("局域网服务地址应解析到本地文件 %s，实际 %s（err=%v）", abs, resolved, err)
	}
	local, cleanup, err := normalizeAdmitSource(AdmitInput{SourceKind: SourceKindURL, Source: source})
	if err != nil || local != abs || cleanup != nil {
		t.Fatalf("URL 来源应直接复用本地文件，实际 %q（cleanup=%v, err=%v）", local, cleanup != nil, err)
	}
}

func TestIsLocalVideoURL(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	rel := "multimedia/33bfb0e20c938527.mp4"
	abs := filepath.Join(config.LocalDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fake-video"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"/file/read/" + rel,
		config.LocalBaseURL() + "/file/read/" + rel,
	} {
		if !IsLocalVideoURL(source) {
			t.Fatalf("%s 应识别为本机视频", source)
		}
	}
	if IsLocalVideoURL("/file/read/multimedia/not-video.jpg") {
		t.Fatal("图片不应识别为本机视频")
	}
}

// TestDownloadToTempLimited 远程资源回源：正常小文件可下载，超限时明确报错且不留临时文件。
func TestDownloadToTempLimited(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)

	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("resource-bytes"))
	}))
	defer small.Close()

	local, cleanup, err := downloadToTempLimited(small.URL, "sample.bin", 1024)
	if err != nil {
		t.Fatalf("正常小文件应下载成功: %v", err)
	}
	if cleanup == nil {
		t.Fatal("临时下载件必须返回清理函数")
	}
	data, readErr := os.ReadFile(local)
	if readErr != nil || string(data) != "resource-bytes" {
		t.Fatalf("下载内容不正确: %q, err=%v", data, readErr)
	}
	cleanup()
	if _, statErr := os.Stat(local); !os.IsNotExist(statErr) {
		t.Fatal("cleanup 后临时文件应被删除")
	}

	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer big.Close()

	path, bigCleanup, err := downloadToTempLimited(big.URL, "big.bin", 64)
	if err == nil || !strings.Contains(err.Error(), "资源过大") {
		t.Fatalf("超限应返回明确的超限错误，实际 %v", err)
	}
	if path != "" || bigCleanup != nil {
		t.Fatal("超限失败时不得返回路径或清理函数")
	}
	entries, readDirErr := os.ReadDir(tempDir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	if len(entries) != 0 {
		t.Fatalf("超限失败后残留临时文件: %v", entries)
	}
}

// TestUpgradeCDNScheme 腾讯图床的 http 直链升级为 https；其他地址不得改动。
func TestUpgradeCDNScheme(t *testing.T) {
	cases := map[string]string{
		"http://gchat.qpic.cn/gchatpic_new/0/0-0-abc/0?term=2": "https://gchat.qpic.cn/gchatpic_new/0/0-0-abc/0?term=2",
		"http://multimedia.nt.qq.com.cn/download?fileid=abc":   "https://multimedia.nt.qq.com.cn/download?fileid=abc",
		"https://gchat.qpic.cn/gchatpic_new/0/0-0-abc/0":       "https://gchat.qpic.cn/gchatpic_new/0/0-0-abc/0",
		"http://127.0.0.1:36789/file/read/multimedia/a.jpg":    "http://127.0.0.1:36789/file/read/multimedia/a.jpg",
		"http://example.com/a.jpg":                             "http://example.com/a.jpg",
		"data:image/png;base64,AAAA":                           "data:image/png;base64,AAAA",
	}
	for input, want := range cases {
		if got := UpgradeCDNScheme(input); got != want {
			t.Fatalf("UpgradeCDNScheme(%q) = %q, want %q", input, got, want)
		}
	}
}
