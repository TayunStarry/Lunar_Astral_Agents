package resource

// register_test.go 内容仓库登记入口的返回值契约
//
// RegisterPath 的返回顺序是 (canonicalRel, handle)：调用方把 canonicalRel 拼进
// /file/read/ 交给前端与 QQ 适配器，把 handle 写进上下文。顺序写反时，图片地址会变成
// /file/read/@I.00x → 宿主 404 → NapCat 报「下载文件失败: Not Found」。
// 两种返回值都是非空字符串，类型系统无法区分，只能靠测试把顺序钉住。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegisterPathReturnsCanonicalPathAndHandle(t *testing.T) {
	tmp := setupArchiveTest(t)
	rel := "multimedia/generated/regression.png"
	abs := filepath.Join(tmp, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatalf("创建生成目录失败: %v", err)
	}
	if err := os.WriteFile(abs, []byte("fake-png-bytes"), 0644); err != nil {
		t.Fatalf("写入测试图片失败: %v", err)
	}

	canonical, handle, err := RegisterPath(rel)
	if err != nil {
		t.Fatalf("登记内容仓库路径失败: %v", err)
	}
	if canonical != rel {
		t.Fatalf("canonical 应为内容仓库相对路径，实得 %q", canonical)
	}
	if !strings.HasPrefix(handle, "@I.") {
		t.Fatalf("handle 应为图片句柄，实得 %q", handle)
	}

	// UpsertPath 只取句柄：与 RegisterPath 的第二个返回值必须一致
	direct, err := UpsertPath(rel)
	if err != nil {
		t.Fatalf("按路径补登记句柄失败: %v", err)
	}
	if direct != handle {
		t.Fatalf("UpsertPath 句柄不一致: %q vs %q", direct, handle)
	}
}
