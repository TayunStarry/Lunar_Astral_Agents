package config

// release_test.go 独立发布配置文件（local_data/release.json）的回归测试：
// 文件缺失时安全返回、存在时读取并写入全局发布配置（含 gitee_token）。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReleaseConfigFile(t *testing.T) {
	dir := t.TempDir()

	// 文件不存在：返回 (nil, false, nil)，不报错
	if cfg, ok, err := LoadReleaseConfigFile(dir); err != nil || ok || cfg != nil {
		t.Fatalf("缺失文件应返回 nil/false/nil，实得 cfg=%v ok=%v err=%v", cfg, ok, err)
	}

	raw := `{
		"gitee_owner": "TayunStarry",
		"gitee_repo": "Lunar-Astral-Agents",
		"gitee_token": "token-in-file",
		"include": ["local_data/package"],
		"program_files": ["Lunar_Astral_Agents.exe"],
		"backup_keep": 3,
		"target_dir": ".",
		"package_dir": "local_data/package"
	}`
	// 先备份全局值，测试结束后还原，避免污染其它用例
	oldOwner, oldRepo, oldToken := ReleaseGiteeOwner, ReleaseGiteeRepo, ReleaseGiteeToken
	oldInclude, oldPrograms, oldKeep := ReleaseInclude, ReleaseProgramFiles, ReleaseBackupKeep
	t.Cleanup(func() {
		ReleaseGiteeOwner, ReleaseGiteeRepo, ReleaseGiteeToken = oldOwner, oldRepo, oldToken
		ReleaseInclude, ReleaseProgramFiles, ReleaseBackupKeep = oldInclude, oldPrograms, oldKeep
	})

	if err := os.WriteFile(filepath.Join(dir, ReleaseConfigFile), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, ok, err := LoadReleaseConfigFile(dir)
	if err != nil || !ok || cfg == nil {
		t.Fatalf("读取 release.json 失败: ok=%v err=%v", ok, err)
	}
	if cfg.GiteeToken != "token-in-file" {
		t.Fatalf("gitee_token 应被解析，实得 %q", cfg.GiteeToken)
	}

	applyReleaseConfig(cfg)
	if ReleaseGiteeToken != "token-in-file" {
		t.Fatalf("令牌应写入全局配置，实得 %q", ReleaseGiteeToken)
	}
	if ReleaseGiteeOwner != "TayunStarry" || ReleaseGiteeRepo != "Lunar-Astral-Agents" {
		t.Fatalf("仓库信息未正确写入: %s/%s", ReleaseGiteeOwner, ReleaseGiteeRepo)
	}
	if len(ReleaseInclude) != 1 || ReleaseInclude[0] != "local_data/package" {
		t.Fatalf("include 未正确写入: %#v", ReleaseInclude)
	}
	if ReleaseBackupKeep == nil || *ReleaseBackupKeep != 3 {
		t.Fatalf("backup_keep 未正确写入: %#v", ReleaseBackupKeep)
	}
}
