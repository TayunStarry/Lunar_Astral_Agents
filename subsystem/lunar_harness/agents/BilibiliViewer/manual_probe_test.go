package BilibiliViewer

// manual_probe_test.go 手动探针共用的数据目录解析。

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// localDataEnv 手动探针覆盖数据目录的环境变量。
const localDataEnv = "LUNAR_LOCAL_DATA"

// manualLocalDataDir 返回手动探针使用的 local_data 绝对路径：
// 优先读环境变量，未设置时按测试文件位置推导仓库根目录下的 local_data。
func manualLocalDataDir(t *testing.T) string {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv(localDataEnv))
	if dir == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("无法定位手动探针测试文件")
		}
		// 当前文件位于 subsystem/lunar_harness/agents/BilibiliViewer，向上四级即仓库根。
		dir = filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "local_data")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", localDataEnv, err)
	}
	return abs
}
