package napcat

// fetch_media_test.go 渠道媒体回源：大小上限与非 2xx 处理。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadBytesLimited(t *testing.T) {
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("media-bytes"))
	}))
	defer small.Close()

	data, err := downloadBytesLimited(small.URL, 1024)
	if err != nil {
		t.Fatalf("正常小文件应下载成功: %v", err)
	}
	if string(data) != "media-bytes" {
		t.Fatalf("媒体内容不正确: %q", data)
	}

	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer big.Close()

	data, err = downloadBytesLimited(big.URL, 64)
	if err == nil || !strings.Contains(err.Error(), "资源过大") {
		t.Fatalf("超限应返回明确的超限错误，实际 %v", err)
	}
	if data != nil {
		t.Fatal("超限失败时不得返回截断内容")
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer broken.Close()

	if _, err := downloadBytesLimited(broken.URL, 1024); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("非 2xx 应返回明确状态错误，实际 %v", err)
	}
}
