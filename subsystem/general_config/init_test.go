package GeneralConfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetReleaseConfig 复位发布通道全局变量（applyConfig 会写全局，用例间需隔离）
func resetReleaseConfig() {
	ReleaseGiteeOwner = ""
	ReleaseGiteeRepo = ""
	ReleaseGiteeToken = ""
	ReleaseInclude = nil
	ReleaseExclude = nil
	ReleaseExcludeDirsWith = nil
	ReleaseBackupKeep = nil
	ReleaseProgramFiles = nil
	ReleaseRestartArgs = nil
	ReleaseTargetDir = ""
	ReleasePackageDir = ""
}

// TestApplyConfigReleaseSection release 节：更新包与 LTPX 市场共用一套仓库配置
func TestApplyConfigReleaseSection(t *testing.T) {
	resetReleaseConfig()
	defer resetReleaseConfig()

	parameter := &ModelConfig{}
	parameter.Release = &struct {
		GiteeOwner      string   `json:"gitee_owner"`
		GiteeRepo       string   `json:"gitee_repo"`
		GiteeToken      string   `json:"gitee_token"`
		Include         []string `json:"include"`
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		BackupKeep      *int     `json:"backup_keep,omitempty"`
		ProgramFiles    []string `json:"program_files"`
		RestartArgs     []string `json:"restart_args"`
		TargetDir       string   `json:"target_dir"`
		PackageDir      string   `json:"package_dir"`
	}{}
	parameter.Release.GiteeOwner = "owner"
	parameter.Release.GiteeRepo = "repo"
	parameter.Release.Include = []string{"local_data/tools"}
	parameter.Release.ProgramFiles = []string{"Lunar_Astral_Agents.exe"}
	parameter.Release.ExcludeDirsWith = []string{"metadata.json"}
	parameter.Release.TargetDir = "."
	parameter.Release.PackageDir = "local_data/package"
	keep := 5
	parameter.Release.BackupKeep = &keep

	legacy := applyConfig(parameter)
	if len(legacy) != 0 {
		t.Fatalf("仅配置 release 节时不应触发旧节迁移提示，实际 %v", legacy)
	}
	if ReleaseGiteeOwner != "owner" || ReleaseGiteeRepo != "repo" {
		t.Fatalf("仓库未写入：%q/%q", ReleaseGiteeOwner, ReleaseGiteeRepo)
	}
	if len(ReleaseInclude) != 1 || ReleaseInclude[0] != "local_data/tools" {
		t.Fatalf("include 未写入：%v", ReleaseInclude)
	}
	if ReleaseBackupKeep == nil || *ReleaseBackupKeep != 5 {
		t.Fatalf("backup_keep 未写入：%v", ReleaseBackupKeep)
	}
	if ReleasePackageDir != "local_data/package" {
		t.Fatalf("package_dir 未写入：%q", ReleasePackageDir)
	}
}

// TestApplyConfigLegacyReleaseSections 旧配置节回退：update_pack / ltpx_market 仍可用，
// 且必须给出迁移提示（否则用户升级后配置静默失效而无人察觉）
func TestApplyConfigLegacyReleaseSections(t *testing.T) {
	resetReleaseConfig()
	defer resetReleaseConfig()

	parameter := &ModelConfig{}
	parameter.LegacyUpdatePack = &struct {
		GiteeOwner      string   `json:"gitee_owner"`
		GiteeRepo       string   `json:"gitee_repo"`
		GiteeToken      string   `json:"gitee_token"`
		Include         []string `json:"include"`
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		BackupKeep      *int     `json:"backup_keep"`
		ProgramFiles    []string `json:"program_files"`
		RestartArgs     []string `json:"restart_args"`
		TargetDir       string   `json:"target_dir"`
	}{}
	parameter.LegacyUpdatePack.GiteeOwner = "legacy-owner"
	parameter.LegacyUpdatePack.GiteeRepo = "legacy-repo"
	parameter.LegacyUpdatePack.Include = []string{"local_data/audios"}
	parameter.LegacyLtpxMarket = &struct {
		GiteeOwner string `json:"gitee_owner"`
		GiteeRepo  string `json:"gitee_repo"`
		PackageDir string `json:"package_dir"`
	}{}
	parameter.LegacyLtpxMarket.PackageDir = "local_data/packages"

	legacy := applyConfig(parameter)
	if len(legacy) != 1 || !strings.Contains(legacy[0], "release") {
		t.Fatalf("应给出旧节迁移提示，实际 %v", legacy)
	}
	if ReleaseGiteeOwner != "legacy-owner" || ReleaseGiteeRepo != "legacy-repo" {
		t.Fatalf("旧节仓库未回退读取：%q/%q", ReleaseGiteeOwner, ReleaseGiteeRepo)
	}
	if len(ReleaseInclude) != 1 || ReleaseInclude[0] != "local_data/audios" {
		t.Fatalf("旧节 include 未回退读取：%v", ReleaseInclude)
	}
	if ReleasePackageDir != "local_data/packages" {
		t.Fatalf("旧节 package_dir 未回退读取：%q", ReleasePackageDir)
	}
}

// TestApplyConfigNewSectionWins 同时存在新旧节时以新节为准
func TestApplyConfigNewSectionWins(t *testing.T) {
	resetReleaseConfig()
	defer resetReleaseConfig()

	parameter := &ModelConfig{}
	parameter.Release = &struct {
		GiteeOwner      string   `json:"gitee_owner"`
		GiteeRepo       string   `json:"gitee_repo"`
		GiteeToken      string   `json:"gitee_token"`
		Include         []string `json:"include"`
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		BackupKeep      *int     `json:"backup_keep,omitempty"`
		ProgramFiles    []string `json:"program_files"`
		RestartArgs     []string `json:"restart_args"`
		TargetDir       string   `json:"target_dir"`
		PackageDir      string   `json:"package_dir"`
	}{}
	parameter.Release.GiteeOwner = "new-owner"
	parameter.Release.GiteeRepo = "new-repo"

	parameter.LegacyUpdatePack = &struct {
		GiteeOwner      string   `json:"gitee_owner"`
		GiteeRepo       string   `json:"gitee_repo"`
		GiteeToken      string   `json:"gitee_token"`
		Include         []string `json:"include"`
		Exclude         []string `json:"exclude"`
		ExcludeDirsWith []string `json:"exclude_dirs_with"`
		BackupKeep      *int     `json:"backup_keep"`
		ProgramFiles    []string `json:"program_files"`
		RestartArgs     []string `json:"restart_args"`
		TargetDir       string   `json:"target_dir"`
	}{}
	parameter.LegacyUpdatePack.GiteeOwner = "old-owner"

	// 新节存在时旧节整体不生效（旧节内容不得覆盖新节）
	_ = applyConfig(parameter)
	if ReleaseGiteeOwner != "new-owner" || ReleaseGiteeRepo != "new-repo" {
		t.Fatalf("应以新节为准，实际 %q/%q", ReleaseGiteeOwner, ReleaseGiteeRepo)
	}
}

// TestApplyConfigSmallGroupsMerged 零碎小节归并：multimodal → agent（旧节回退 + 迁移提示）
func TestApplyConfigSmallGroupsMerged(t *testing.T) {
	resetReleaseConfig()
	defer resetReleaseConfig()
	oldVideo := *VideoInputMode
	defer func() { *VideoInputMode = oldVideo }()

	// 新位置
	parameter := &ModelConfig{}
	parameter.Agent.VideoInput = "frames"
	legacy := applyConfig(parameter)
	if len(legacy) != 0 {
		t.Fatalf("使用新位置时不应有迁移提示：%v", legacy)
	}
	if *VideoInputMode != "frames" {
		t.Fatalf("agent.video_input 未生效：%q", *VideoInputMode)
	}

	// 旧位置（回退 + 提示）
	resetReleaseConfig()
	*VideoInputMode = "file"
	parameter2 := &ModelConfig{}
	parameter2.LegacyMultimodal.VideoInput = "frames"
	legacy2 := applyConfig(parameter2)
	if *VideoInputMode != "frames" {
		t.Fatalf("旧 multimodal.video_input 未回退读取：%q", *VideoInputMode)
	}
	joined := strings.Join(legacy2, " | ")
	if !strings.Contains(joined, "agent.video_input") {
		t.Fatalf("迁移提示缺少 agent.video_input：%s", joined)
	}
}

// TestApplyConfigLegacyTUISectionWarns 旧 tui 节已随桌面版取消：
// 配置仍含该节时必须给出明确提示（而不是静默忽略，让人以为配置没生效）
func TestApplyConfigLegacyTUISectionWarns(t *testing.T) {
	resetReleaseConfig()
	defer resetReleaseConfig()

	disabled := false
	parameter := &ModelConfig{}
	parameter.LegacyTUI.ExitWhenFrontendsClosed = &disabled
	legacy := applyConfig(parameter)
	joined := strings.Join(legacy, " | ")
	if !strings.Contains(joined, "tui.exit_when_frontends_closed") || !strings.Contains(joined, "已取消") {
		t.Fatalf("旧 tui 节应提示已取消，实际 %v", legacy)
	}
}

// configSectionAllowlist 出厂配置允许出现的顶层节。
// 写错节名（或仍写在已废弃的旧节里）会被 json 静默忽略——配置项静默失效是最难排查的
// 一类问题，故用白名单把「未知顶层键」钉死在测试里。
var configSectionAllowlist = map[string]bool{
	"models": true, "server": true, "agent": true, "memory": true, "chat": true,
	"diffusion": true, "bridging": true, "tts": true, "web_search": true,
	"client": true, "release": true,
}

// TestShippedConfigParses 出厂 lunar_config.json 必须与当前结构体契约一致：
// 能被完整解析、关键节已就位、且不含已废弃的旧节与拼错的顶层键。
func TestShippedConfigParses(t *testing.T) {
	path := filepath.Join("..", "..", "local_data", "lunar_config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取出厂配置失败: %v", err)
	}

	parameter := &ModelConfig{}
	if err := json.Unmarshal(data, parameter); err != nil {
		t.Fatalf("出厂配置解析失败: %v", err)
	}

	if parameter.Release == nil {
		t.Fatalf("出厂配置缺少 release 节（更新包与 LTPX 市场共用的发布通道）")
	}
	if parameter.Release.GiteeOwner == "" || parameter.Release.GiteeRepo == "" {
		t.Fatalf("release 节缺少仓库信息：%q/%q", parameter.Release.GiteeOwner, parameter.Release.GiteeRepo)
	}
	if len(parameter.Release.Include) == 0 || len(parameter.Release.ProgramFiles) == 0 {
		t.Fatalf("release 节缺少打包清单：include=%v program_files=%v",
			parameter.Release.Include, parameter.Release.ProgramFiles)
	}
	if parameter.Agent.VideoInput != "file" {
		t.Fatalf("agent.video_input 应为 file，实际 %q", parameter.Agent.VideoInput)
	}

	// 旧节必须清空：留着会让「旧节不生效」的告警长期存在
	for name, present := range map[string]bool{
		"update_pack": parameter.LegacyUpdatePack != nil,
		"ltpx_market": parameter.LegacyLtpxMarket != nil,
		"tui":         parameter.LegacyTUI.ExitWhenFrontendsClosed != nil,
		"multimodal":  parameter.LegacyMultimodal.VideoInput != "",
	} {
		if present {
			t.Fatalf("出厂配置不应再包含已废弃的 %s 节（已并入 release / server / agent）", name)
		}
	}

	// 未知顶层键检测
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("按原始键解析失败: %v", err)
	}
	for key := range raw {
		if strings.HasPrefix(key, "-") { // 配置内的注释键（如 -enumerate）不算配置项
			continue
		}
		if !configSectionAllowlist[key] {
			t.Fatalf("出厂配置含未知顶层节 %q：写错节名会被静默忽略，请核对结构体契约", key)
		}
	}
}
