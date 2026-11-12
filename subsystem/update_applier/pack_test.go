package main

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// repoSevenZip 返回仓库内自带的 7z（用于真实打包测试）；找不到时跳过。
func repoSevenZip(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(filepath.Clean(filepath.Join(wd, "..", "..")), "local_data", "package", "archive", "7z.exe")
	if !fileExists(path) {
		t.Skip("仓库内未找到 7z.exe，跳过真实打包测试")
	}
	return path
}

func TestBuildUpdatePackageRealVolumes(t *testing.T) {
	sevenZip := repoSevenZip(t)
	root := t.TempDir()

	// 程序文件与资源
	writeTestFile(t, filepath.Join(root, "Lunar_Astral_Agents.exe"), "main-binary")
	writeTestFile(t, filepath.Join(root, "Lunar_Update_Applier.exe"), "updater-binary")
	writeTestFile(t, filepath.Join(root, "local_data", "audios", "voice.wav"), string(bytes.Repeat([]byte("a"), 100*1024)))
	// 伪随机内容不可压缩，确保按 1MB 真实分卷。
	random := make([]byte, 2200*1024)
	rng := rand.New(rand.NewSource(20261111))
	if _, err := rng.Read(random); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "local_data", "package", "big.bin"), string(random))
	writeTestFile(t, filepath.Join(root, "local_data", "package", "core", "readme.txt"), "core")
	writeTestFile(t, filepath.Join(root, "local_data", "config", "settings.json"), `{"ok":true}`)
	// 必须被排除的内容
	writeTestFile(t, filepath.Join(root, "local_data", "config", "release.json"), `{"token":"secret"}`)
	writeTestFile(t, filepath.Join(root, "local_data", "config", "version.json"), `{"version":"v1"}`)
	writeTestFile(t, filepath.Join(root, "local_data", "package", "tool", "metadata.json"), `{}`)
	writeTestFile(t, filepath.Join(root, "local_data", "package", "tool", "keep.txt"), "should-not-pack")

	cfg := &ReleaseConfig{
		Include:         []string{"local_data/package", "local_data/audios", "local_data/config"},
		ProgramFiles:    []string{"Lunar_Astral_Agents.exe", "Lunar_Update_Applier.exe"},
		ExcludeDirsWith: []string{"metadata.json"},
		PackSizeMB:      1,
		PackLevel:       1,
		SevenZip:        sevenZip,
	}

	outDir := filepath.Join(root, ".update", "pack", "test")
	result, err := BuildUpdatePackage(root, cfg, "v2026.11.11-1530", outDir, nil)
	if err != nil {
		t.Fatalf("真实打包失败: %v", err)
	}
	if len(result.Volumes) < 2 {
		t.Fatalf("2.2MB 数据按 1MB 分卷应产生多卷，实际 %d 卷", len(result.Volumes))
	}
	if len(result.SHAs) != len(result.Volumes) {
		t.Fatalf("校验值数量与分卷不一致: %d/%d", len(result.SHAs), len(result.Volumes))
	}
	for _, vol := range result.Volumes {
		sum, err := hashFileSHA256(vol)
		if err != nil {
			t.Fatal(err)
		}
		if result.SHAs[filepath.Base(vol)] != sum {
			t.Fatalf("分卷 %s 的 SHA 不一致", vol)
		}
	}

	got := make(map[string]bool)
	for _, f := range result.Manifest.Files {
		got[f.Path] = true
		sum, err := hashFileSHA256(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil || sum != f.SHA256 {
			t.Fatalf("清单哈希不一致: %s (%v)", f.Path, err)
		}
	}
	want := []string{
		"Lunar_Astral_Agents.exe",
		"Lunar_Update_Applier.exe",
		"local_data/audios/voice.wav",
		"local_data/package/big.bin",
		"local_data/package/core/readme.txt",
		"local_data/config/settings.json",
	}
	for _, p := range want {
		if !got[p] {
			t.Fatalf("清单缺少文件: %s", p)
		}
	}
	for _, p := range []string{
		"local_data/config/release.json",
		"local_data/config/version.json",
		"local_data/package/tool/keep.txt",
		"local_data/package/tool/metadata.json",
	} {
		if got[p] {
			t.Fatalf("排除规则失效，清单包含: %s", p)
		}
	}
	if result.Manifest.Version != "v2026.11.11-1530" {
		t.Fatalf("清单版本 = %s", result.Manifest.Version)
	}
	if fileExists(filepath.Join(root, manifestFileName)) {
		t.Fatal("打包后应在程序根移除临时清单")
	}

	// 用 7z 测试并解压，确认分卷真实可读、清单可通过校验。
	if err := testArchive(sevenZip, result.Volumes[0]); err != nil {
		t.Fatalf("7z 完整性测试失败: %v", err)
	}
	staging := t.TempDir()
	if err := extractVolumes(sevenZip, result.Volumes[0], staging); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	files, err := verifyExtracted(staging, "v2026.11.11-1530", nil)
	if err != nil {
		t.Fatalf("解压内容校验失败: %v", err)
	}
	if len(files) != len(result.Manifest.Files) {
		t.Fatalf("解压文件数 = %d，清单 = %d", len(files), len(result.Manifest.Files))
	}
}

func TestParseDeclaredSHAs(t *testing.T) {
	body := "[UPDATE]\n**包内文件与校验**\n- a.7z.001（1.0 MB）\n  SHA-256: ABCDEF\n- b.7z.002（1.0 MB）\n  SHA-256: 123456\n"
	got := parseDeclaredSHAs(body)
	if got["a.7z.001"] != "abcdef" || got["b.7z.002"] != "123456" {
		t.Fatalf("解析结果 = %+v", got)
	}
}

func TestCompareTagNaturalAndRelation(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v2026.11.11-1530", "v2026.11.12-0900", -1},
		{"v2026.11.12-0900", "v2026.11.11-1530", 1},
		{"v2026.11.11-1530", "v2026.11.11-1530", 0},
	}
	for _, c := range cases {
		if got := CompareTagNatural(c.a, c.b); got != c.want {
			t.Fatalf("CompareTagNatural(%s,%s) = %d", c.a, c.b, got)
		}
	}
	if VersionRelation("v10.0.0", "v9.9.9") != relationOlder {
		t.Fatal("降级关系判断错误")
	}
	if VersionRelation("", "v1") != relationUnknown {
		t.Fatal("未知版本应返回 unknown")
	}
}
