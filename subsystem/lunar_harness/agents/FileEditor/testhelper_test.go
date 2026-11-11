package FileEditor

// testhelper_test.go 文档库读写测试共用的隔离环境。

import (
	"os"
	"path/filepath"
	"testing"

	"LunarSubsystem/LunarHarness/internal/testenv"
)

// prepareScheduleDir 准备隔离的文档库与句柄库。
func prepareScheduleDir(t *testing.T) {
	t.Helper()
	root := testenv.Isolate(t)
	dir := filepath.Join(root, "documents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
}
