package main

import (
	"math/rand"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublishChainLightSample 轻量跑通「发布」链路：
// 枚举 → 清单 → 哈希 → 7z 分卷 → 发行说明 → 上传请求构造。
//
// 不联网：上传只校验请求地址与 multipart 之前的构造；Release 复用解析用构造数据模拟。
// 样本：4 个文件（3 个小文本 + 1 个 1.2MB 不可压缩数据），1MB 分卷 → 2 卷。
func TestPublishChainLightSample(t *testing.T) {
	sevenZip := repoSevenZip(t)
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "Lunar_Astral_Agents.exe"), "main-binary")
	writeTestFile(t, filepath.Join(root, "local_data", "config", "settings.json"), `{"ok":true}`)
	writeTestFile(t, filepath.Join(root, "local_data", "audios", "tone.wav"), "tone")

	random := make([]byte, 1200*1024)
	rng := rand.New(rand.NewSource(20261112))
	if _, err := rng.Read(random); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "local_data", "package", "blob.bin"), string(random))

	// 不得进入清单的配置
	writeTestFile(t, filepath.Join(root, "local_data", "config", "release.json"), `{"token":"secret"}`)
	writeTestFile(t, filepath.Join(root, "local_data", "config", "version.json"), `{"version":"v1"}`)

	cfg := &ReleaseConfig{
		GiteeOwner:   "owner",
		GiteeRepo:    "repo",
		GiteeToken:   "tok en/+",
		Include:      []string{"local_data/package", "local_data/audios", "local_data/config"},
		ProgramFiles: []string{"Lunar_Astral_Agents.exe"},
		PackSizeMB:   1,
		PackLevel:    1,
		SevenZip:     sevenZip,
	}
	version := "v2026.11.12-0900"

	outDir := filepath.Join(root, ".update", "pack", "sample")
	result, err := BuildUpdatePackage(root, cfg, version, outDir, nil)
	if err != nil {
		t.Fatalf("打包失败: %v", err)
	}
	if result.FileCount != 4 || len(result.Manifest.Files) != 4 {
		t.Fatalf("清单文件数 = %d", result.FileCount)
	}
	if len(result.Volumes) < 2 {
		t.Fatalf("1.2MB 数据按 1MB 分卷应产生多卷，实际 %d 卷", len(result.Volumes))
	}
	t.Logf("样本：%d 个文件 / %s；产出 %d 个分卷", result.FileCount, humanSize(result.Bytes), len(result.Volumes))
	for _, vol := range result.Volumes {
		name := filepath.Base(vol)
		sum, err := hashFileSHA256(vol)
		if err != nil {
			t.Fatalf("计算分卷哈希失败: %v", err)
		}
		if result.SHAs[name] != sum {
			t.Fatalf("分卷 %s 哈希与产物不一致", name)
		}
		t.Logf("  %s（%s）SHA-256 %s", name, humanSize(fileSize(vol)), sum)
	}

	// 发行说明：标记、版本、每个分卷与 SHA 都要能回读。
	body := buildReleaseBody(cfg, result)
	if !strings.Contains(body, updateMarker) || !strings.Contains(body, version) {
		t.Fatalf("发行说明缺少标记或版本:\n%s", body)
	}
	declared := parseDeclaredSHAs(body)
	for _, vol := range result.Volumes {
		name := filepath.Base(vol)
		if declared[name] != result.SHAs[name] {
			t.Fatalf("发行说明中的 SHA 与产物不一致: %s", name)
		}
	}

	// 模拟 Gitee 返回的 Release，确认发布端产出的说明能被安装端解析为「可用」。
	raw := GiteeRelease{ID: 42, TagName: version, Name: "更新包 " + version, Body: body}
	for _, vol := range result.Volumes {
		name := filepath.Base(vol)
		raw.Assets = append(raw.Assets, GiteeAsset{
			ID: int64(len(raw.Assets) + 1), Name: name, Size: fileSize(vol),
			BrowserDownloadURL: "https://example.invalid/" + name,
		})
	}
	parsed := parseUpdateRelease(raw)
	if !parsed.Valid || parsed.Problem != "" {
		t.Fatalf("安装端判定发布不可用: %s", parsed.Problem)
	}
	if len(parsed.Volumes) != len(result.Volumes) {
		t.Fatalf("分卷数不一致: %d/%d", len(parsed.Volumes), len(result.Volumes))
	}

	// 上传请求构造：地址包含仓库、Release ID 与转义后的令牌。
	uploadURL := uploadAssetURL(cfg, 42)
	for _, want := range []string{"/repos/owner/repo/releases/42/attach_files", "access_token=tok+en%2F%2B"} {
		if !strings.Contains(uploadURL, want) {
			t.Fatalf("上传地址缺少 %q: %s", want, uploadURL)
		}
	}

	// 打包往返：分卷可测、可解，解压内容与清单一致。
	if err := testArchive(sevenZip, result.Volumes[0]); err != nil {
		t.Fatalf("7z 完整性测试失败: %v", err)
	}
	staging := t.TempDir()
	if err := extractVolumes(sevenZip, result.Volumes[0], staging); err != nil {
		t.Fatalf("解压失败: %v", err)
	}
	files, err := verifyExtracted(staging, version, nil)
	if err != nil {
		t.Fatalf("解压内容校验失败: %v", err)
	}
	if len(files) != result.FileCount {
		t.Fatalf("解压文件数 = %d，清单 = %d", len(files), result.FileCount)
	}
	for _, f := range files {
		if strings.HasSuffix(f.Path, "release.json") || strings.HasSuffix(f.Path, "version.json") {
			t.Fatalf("解压清单包含应排除的配置文件: %s", f.Path)
		}
	}
}
