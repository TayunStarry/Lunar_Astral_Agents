package resource

// unknown_test.go local_data/unknown 纳入句柄扫描与 GC 的验收：
// 不可读二进制文件（zip / 7z / exe / dll / gguf…）各自获得独立句柄，实体丢失后句柄被清理。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnknownStorageRootRegistersBinaries(t *testing.T) {
	tmp := setupArchiveTest(t)

	unknownDir := filepath.Join(tmp, StorageRootUnknown)
	if err := os.MkdirAll(unknownDir, 0o755); err != nil {
		t.Fatalf("创建 unknown 目录失败: %v", err)
	}
	files := map[string][]byte{
		"pkg.zip":    []byte("PK\x03\x04 binary zip content"),
		"archive.7z": []byte("7z\xbc\xaf\x27\x1c binary 7z content"),
		"tool.exe":   []byte("MZ binary executable"),
		"model.gguf": []byte("GGUF binary model"),
		"plugin.dll": []byte("MZ binary library"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(unknownDir, name), data, 0o644); err != nil {
			t.Fatalf("写入 %s 失败: %v", name, err)
		}
	}

	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}
	handles := map[string]bool{}
	for name := range files {
		res, ok := ForPath(StorageRootUnknown + "/" + name)
		if !ok {
			t.Fatalf("%s 未纳入句柄扫描", name)
		}
		if res.Kind != KindDocument || res.Handle == "" {
			t.Fatalf("%s 句柄异常: kind=%s handle=%q", name, res.Kind, res.Handle)
		}
		if handles[res.Handle] {
			t.Fatalf("%s 与其他文件复用了同一个句柄 %s", name, res.Handle)
		}
		handles[res.Handle] = true
	}

	// 二进制文件仍用 @D 句柄，但取回时必须走 base64，而不是把字节当文本返回。
	pkg, ok := ForPath(StorageRootUnknown + "/pkg.zip")
	if !ok {
		t.Fatal("pkg.zip 未登记句柄")
	}
	read, err := Read(pkg.Handle, 4096)
	if err != nil {
		t.Fatalf("读取二进制句柄失败: %v", err)
	}
	if read.Content != "" || read.Base64 == "" {
		t.Fatalf("二进制资源应按 base64 取回: content=%q base64_len=%d", read.Content, len(read.Base64))
	}

	// 删除实体后再次同步：unknown 内的句柄同样要被清理
	if err := os.Remove(filepath.Join(unknownDir, "pkg.zip")); err != nil {
		t.Fatalf("删除 pkg.zip 失败: %v", err)
	}
	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("二次同步失败: %v", err)
	}
	if _, ok := ForPath(StorageRootUnknown + "/pkg.zip"); ok {
		t.Fatal("unknown 目录内实体丢失后句柄应被清理")
	}
}
