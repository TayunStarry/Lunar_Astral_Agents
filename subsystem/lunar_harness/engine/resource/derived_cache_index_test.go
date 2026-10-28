package resource

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDerivedCacheNotIndexed 覆盖：multimedia/vision 与 multimedia/footage 是派生缓存，
// 不得进入句柄索引；历史遗留的目录行应被启动同步清理，但缓存文件本身必须保留
// （vision 副本被缓存表引用、footage 切片可能正被模型读取）。
func TestDerivedCacheNotIndexed(t *testing.T) {
	tmp := setupArchiveTest(t)

	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04,
		0x08, 0x02, 0x00, 0x00, 0x00,
	}
	visionRel := "multimedia/vision/abcd_s1024.png"
	footageRel := "multimedia/footage/ef01_seg0.mp4"
	generatedRel := "multimedia/generated/keep.png"
	for rel, data := range map[string][]byte{
		visionRel:    png,
		footageRel:   []byte("fake-mp4-bytes"),
		generatedRel: png,
	} {
		abs := filepath.Join(tmp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(abs, data, 0644); err != nil {
			t.Fatalf("写入测试文件失败: %v", err)
		}
	}

	// 模拟历史版本：视觉副本曾被登记成图片资源
	visionAbs := filepath.Join(tmp, filepath.FromSlash(visionRel))
	hash, size, err := fileSHA16(visionAbs)
	if err != nil {
		t.Fatalf("计算测试图片哈希失败: %v", err)
	}
	if err := archiveDB.InsertResource(&Resource{
		Kind: KindImage, Name: "abcd_s1024.png", Ext: ".png", MIME: "image/png",
		ContentHash: hash, BlobPath: visionRel, SizeBytes: size, Title: "legacy-vision",
		Origin: OriginStorageScan, Retain: RetainNormal, State: StateActive,
	}); err != nil {
		t.Fatalf("写入历史目录行失败: %v", err)
	}

	if err := SyncStorageHandles(); err != nil {
		t.Fatalf("同步失败: %v", err)
	}

	if _, ok := ForPath(visionRel); ok {
		t.Errorf("multimedia/vision 不应被索引（历史目录行也应清理）")
	}
	if _, ok := ForPath(footageRel); ok {
		t.Errorf("multimedia/footage 不应被索引")
	}
	if _, ok := ForPath(generatedRel); !ok {
		t.Errorf("普通生成目录实体应继续获得句柄")
	}
	if _, err := os.Stat(visionAbs); err != nil {
		t.Errorf("清理目录行不得删除视觉缓存文件: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, filepath.FromSlash(footageRel))); err != nil {
		t.Errorf("跳过索引不得删除视频切片文件: %v", err)
	}

	// 写入路径也不应给派生缓存分配句柄
	if _, handle, err := RegisterPath(visionRel); err != nil || handle != "" {
		t.Errorf("RegisterPath 不应为派生缓存分配句柄: handle=%q err=%v", handle, err)
	}
	if _, handle, err := RegisterPath(footageRel); err != nil || handle != "" {
		t.Errorf("RegisterPath 不应为视频切片分配句柄: handle=%q err=%v", handle, err)
	}
}
