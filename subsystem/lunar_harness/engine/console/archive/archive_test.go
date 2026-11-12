package archive

// archive_test.go 用小样本验证本地归档链路：真实 7z 分卷、排除规则与产物命名。
// 归档与更新包是两条互不相关的链路，这里只验证归档自身，不涉及上传或安装。

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoSevenZip 返回仓库内自带的 7z；找不到时跳过真实打包测试。
func repoSevenZip(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Skip(err)
	}
	path := filepath.Join(filepath.Clean(filepath.Join(wd, "..", "..", "..", "..", "..")), "local_data", "package", "archive", "7z.exe")
	if !fileExists(path) {
		t.Skip("仓库内未找到 7z.exe，跳过真实打包测试")
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestArchiveRealVolumesLightSample(t *testing.T) {
	sevenZip := repoSevenZip(t)
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "Lunar_Astral_Agents.exe"), "main-binary")
	writeFile(t, filepath.Join(root, "local_data", "config", "settings.json"), `{"ok":true}`)
	writeFile(t, filepath.Join(root, "local_data", "audios", "voice.wav"), strings.Repeat("a", 100*1024))
	writeFile(t, filepath.Join(root, "local_data", "logs", "run.log"), "should-not-pack")
	writeFile(t, filepath.Join(root, "local_data", "package", "tool", "metadata.json"), `{}`)
	writeFile(t, filepath.Join(root, "local_data", "package", "tool", "keep.txt"), "should-not-pack")
	random := make([]byte, 1200*1024)
	rng := rand.New(rand.NewSource(20261112))
	if _, err := rng.Read(random); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "local_data", "models", "weights.bin"), string(random))

	outBase := filepath.Join(root, "local_data", "archive", "Lunar-Astral-Agents-Release")
	cfg := &Config{
		Import:          []string{root},
		Exclude:         []string{"*.log", "*.7z", "*.tmp"},
		ExcludeDirsWith: []string{"metadata.json"},
		Archive:         []string{sevenZip},
		Output:          outBase,
		Size:            1,
		Level:           1,
	}
	globalConfig = cfg

	if err := createVolume([]string{root}, cfg.Output, cfg.Size, cfg.Level); err != nil {
		t.Fatalf("真实归档失败: %v", err)
	}

	volumes, err := filepath.Glob(outBase + ".7z*")
	if err != nil || len(volumes) < 2 {
		t.Fatalf("1.2MB 数据按 1MB 分卷应产生多卷，实际 %v（%v）", volumes, err)
	}
	names := make([]string, 0, len(volumes))
	for _, vol := range volumes {
		name := filepath.Base(vol)
		if !strings.HasPrefix(name, "Lunar-Astral-Agents-Release.7z.") {
			t.Fatalf("归档产物命名不符合约定: %s", name)
		}
		if strings.Contains(name, "updatepack") {
			t.Fatalf("归档产物误用了更新包命名: %s", name)
		}
		info, err := os.Stat(vol)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, fmt.Sprintf("%s（%d KB）", name, info.Size()/1024))
	}
	t.Logf("归档样本：4 个可打包文件 / 约 1.3MB；产出 %d 个分卷：%s", len(volumes), strings.Join(names, "、"))

	if out, err := exec.Command(sevenZip, "t", volumes[0]).CombinedOutput(); err != nil {
		t.Fatalf("7z 完整性测试失败: %v: %s", err, out)
	}
	extractDir := t.TempDir()
	if out, err := exec.Command(sevenZip, "x", "-y", "-o"+extractDir, volumes[0]).CombinedOutput(); err != nil {
		t.Fatalf("7z 解压失败: %v: %s", err, out)
	}
	for _, want := range []string{
		"Lunar_Astral_Agents.exe",
		"local_data/config/settings.json",
		"local_data/audios/voice.wav",
		"local_data/models/weights.bin",
	} {
		if !fileExists(filepath.Join(extractDir, filepath.FromSlash(want))) {
			t.Fatalf("归档缺少 %s", want)
		}
	}
	for _, unwanted := range []string{
		"local_data/logs/run.log",
		"local_data/package/tool/metadata.json",
		"local_data/package/tool/keep.txt",
	} {
		if fileExists(filepath.Join(extractDir, filepath.FromSlash(unwanted))) {
			t.Fatalf("排除规则失效，归档包含 %s", unwanted)
		}
	}
}
