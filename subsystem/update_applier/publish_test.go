package main

import (
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
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

// TestPublishUploadFailureSurfaces 走真实 HTTP（本地服务替身）复现上传失败：
//   - 预检读仓库默认分支，且创建 Release 必须使用 main 而不是硬编码 master；
//   - multipart 字段名必须是 file，文件名必须是分卷名；
//   - 上传失败时错误、HTTP 状态与 Gitee 返回体必须同时进入返回值与日志。
func TestPublishUploadFailureSurfaces(t *testing.T) {
	resetLogState(t)
	defer resetLogState(t)

	sevenZip := repoSevenZip(t)
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "app.exe"), "main-binary")
	random := make([]byte, 1200*1024)
	rng := rand.New(rand.NewSource(20261112))
	if _, err := rng.Read(random); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "local_data", "blob.bin"), string(random))

	cfg := &ReleaseConfig{
		GiteeOwner:   "owner",
		GiteeRepo:    "repo",
		GiteeToken:   "fake-token",
		Include:      []string{"local_data"},
		ProgramFiles: []string{"app.exe"},
		PackSizeMB:   1,
		PackLevel:    1,
		SevenZip:     sevenZip,
	}
	version := "v2026.11.12-1530"

	var (
		gotTarget string
		gotFile   string
		gotUpload string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v5/repos/owner/repo":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"full_name":"owner/repo","default_branch":"main","permission":{"push":true,"admin":true}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/releases/tags/"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"404 Not Found"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v5/repos/owner/repo/releases":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			gotTarget, _ = payload["target_commitish"].(string)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":7,"tag_name":"`+version+`"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v5/repos/owner/repo/releases/7/attach_files":
			if f, hdr, err := r.FormFile("file"); err != nil {
				gotFile = "字段 file 解析失败：" + err.Error()
			} else {
				gotFile = hdr.Filename
				_, _ = io.Copy(io.Discard, f)
			}
			gotUpload = r.URL.Query().Get("access_token")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"目标分支 master 不存在"}`)
		default:
			t.Errorf("未预期的请求：%s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	oldBase := giteeAPIBase
	giteeAPIBase = srv.URL + "/api/v5"
	defer func() { giteeAPIBase = oldBase }()

	var progresses []Progress
	res, err := PublishUpdate(context.Background(), root, cfg, version, func(p Progress) {
		progresses = append(progresses, p)
	})
	if err == nil {
		t.Fatal("上传返回 400 时应失败")
	}
	for _, want := range []string{"上传", "HTTP 400", "目标分支 master 不存在"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("错误缺少 %q：%v", want, err)
		}
	}
	if gotTarget != "main" {
		t.Fatalf("创建 Release 的 target_commitish 应为 main，实际 %q", gotTarget)
	}
	if gotFile == "" || strings.Contains(gotFile, "失败") {
		t.Fatalf("multipart 字段 file 未正确构造：%q", gotFile)
	}
	if !strings.HasPrefix(gotFile, "lunar_updatepack_") || !strings.HasSuffix(gotFile, ".7z.001") {
		t.Fatalf("上传文件名应为第一卷，实际 %q", gotFile)
	}
	if gotUpload != "fake-token" {
		t.Fatalf("上传请求缺少 access_token，实际 %q", gotUpload)
	}
	if res == nil || len(res.Volumes) < 2 {
		t.Fatalf("上传失败也应返回已打包产物（便于界面展示逐卷状态），实际 %+v", res)
	}

	var failed *Progress
	for i := range progresses {
		if progresses[i].State == "failed" {
			failed = &progresses[i]
		}
	}
	if failed == nil {
		t.Fatal("应回送一条 failed 分卷进度")
	}
	if failed.Item != 0 || !strings.Contains(failed.Detail, "HTTP 400") {
		t.Fatalf("失败分卷进度不正确：%+v", *failed)
	}

	entries, _ := readLogRing(0)
	var logs strings.Builder
	for _, e := range entries {
		logs.WriteString(e.Line)
		logs.WriteString("\n")
	}
	for _, want := range []string{"创建 Release", "阶段=upload", "HTTP 400", "目标分支 master 不存在", "默认分支 main"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("日志缺少 %q：\n%s", want, logs.String())
		}
	}
}
