package httpapi

// read_test.go /file/read/ 的相对路径解析：完整存储路径、历史裸名兜底与越界拒绝。

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/engine/config"
)

func TestReadHandlerPathResolution(t *testing.T) {
	oldDir := config.LocalDir
	config.LocalDir = t.TempDir()
	t.Cleanup(func() { config.LocalDir = oldDir })

	mediaDir := filepath.Join(config.LocalDir, "multimedia")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "abc123.png"), []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config.LocalDir, "documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.LocalDir, "documents", "note.md"), []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		target string
		status int
		body   string
	}{
		{name: "完整相对路径", target: "/file/read/multimedia/abc123.png", status: 200, body: "png-bytes"},
		{name: "历史裸名兜底", target: "/file/read/abc123.png", status: 200, body: "png-bytes"},
		{name: "裸名不存在", target: "/file/read/missing.png", status: 404},
		{name: "越界路径", target: "/file/read/../abc123.png", status: 403},
		{name: "存储根内越界", target: "/file/read/multimedia/../documents/note.md", status: 403},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			ReadHandler(rec, httptest.NewRequest("GET", tc.target, nil))
			if rec.Code != tc.status {
				t.Fatalf("状态码不正确: 期望 %d，实得 %d", tc.status, rec.Code)
			}
			if tc.body != "" && rec.Body.String() != tc.body {
				t.Fatalf("响应内容不正确: %q", rec.Body.String())
			}
		})
	}
}
