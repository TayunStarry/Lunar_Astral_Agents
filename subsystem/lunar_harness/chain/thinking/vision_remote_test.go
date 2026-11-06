package thinking

// vision_remote_test.go 远程图片回源：上限校验与非 2xx 处理。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadRemoteImageLimit(t *testing.T) {
	small := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer small.Close()

	data, err := readRemoteImage(small.URL, 1024)
	if err != nil {
		t.Fatalf("正常小图片应读取成功: %v", err)
	}
	if string(data) != "image-bytes" {
		t.Fatalf("图片内容不正确: %q", data)
	}

	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 4096)))
	}))
	defer big.Close()

	data, err = readRemoteImage(big.URL, 64)
	if err == nil || !strings.Contains(err.Error(), "资源过大") {
		t.Fatalf("超限应返回明确的超限错误，实际 %v", err)
	}
	if data != nil {
		t.Fatal("超限失败时不得返回半张图")
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer broken.Close()

	if _, err := readRemoteImage(broken.URL, 1024); err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("非 2xx 应返回明确状态错误，实际 %v", err)
	}
}
