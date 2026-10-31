package config

// ==== Gitee Release 发布通道（独立配置文件 local_data/release.json） ====
//
// 更新包与 LTPX 应用市场共用同一仓库：两者都以 Gitee Release 存储，
// 在仓库内以「描述标记 + 附件名前缀」严格区分——
//
//	更新包：[UPDATE] 标记 / lunar_updatepack_*.7z.001
//	市场包：[LTPX]   标记 / ltpx_<id>.ltpx
//
// 因此配置只需一套仓库信息；具体通道隔离由 tui_system/updatepack 与
// tui_system/ltpxmarket 各自实现（互不抓取对方的发布）。
//
// 该文件含 Gitee 私人令牌，已被 .gitignore 排除，绝不入库、不上传。
// lunar_config.json 不再支持 release 节：旧写法会被忽略，请迁移到本文件。

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ReleaseConfigFile 独立发布配置文件名（相对 LocalDir）
const ReleaseConfigFile = "release.json"

// ReleaseConfig 发布通道配置（字段与 release.json 一一对应）
type ReleaseConfig struct {
	GiteeOwner string `json:"gitee_owner"` // Gitee 仓库属主
	GiteeRepo  string `json:"gitee_repo"`  // Gitee 仓库名
	GiteeToken string `json:"gitee_token"` // Gitee 私人令牌（需 Release 读写权限；TUI 发布时会自动预填）
	// ==== 更新包（资源 + 主程序） ====
	Include []string `json:"include"` // 打包包含的资源目录（相对 target_dir）
	// 打包排除规则（空时用默认清单）与「含这些文件的目录整体排除」（空时默认 metadata.json）
	Exclude         []string `json:"exclude"`
	ExcludeDirsWith []string `json:"exclude_dirs_with"`
	// 覆盖前备份的保留次数（nil=默认 3；0 或负数=不备份）
	BackupKeep *int `json:"backup_keep,omitempty"`
	// 主程序文件清单（相对程序根目录）：发布时纳入包，拉取时判定是否走「关程序→应用器→重启」
	ProgramFiles []string `json:"program_files"`
	// 重启主程序时携带的命令行参数
	RestartArgs []string `json:"restart_args"`
	// 资源根目录/程序根目录（空时默认 local_data；程序合并后取「程序根目录」）
	TargetDir string `json:"target_dir"`
	// ==== LTPX 应用市场 ====
	// 本地 LTPX 包根目录（空时默认 local_data/package）
	PackageDir string `json:"package_dir"`
}

// LoadReleaseConfigFile 读取 dir 下的 release.json；文件不存在时返回 (nil,false,nil)。
func LoadReleaseConfigFile(dir string) (*ReleaseConfig, bool, error) {
	path := filepath.Join(dir, ReleaseConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var cfg ReleaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, false, err
	}
	return &cfg, true, nil
}

// applyReleaseConfig 把一份发布配置写入全局变量。
func applyReleaseConfig(r *ReleaseConfig) {
	if r == nil {
		return
	}
	ReleaseGiteeOwner = r.GiteeOwner
	ReleaseGiteeRepo = r.GiteeRepo
	ReleaseGiteeToken = r.GiteeToken
	if r.Include != nil {
		ReleaseInclude = r.Include
	}
	if r.Exclude != nil {
		ReleaseExclude = r.Exclude
	}
	if r.ExcludeDirsWith != nil {
		ReleaseExcludeDirsWith = r.ExcludeDirsWith
	}
	if r.BackupKeep != nil {
		ReleaseBackupKeep = r.BackupKeep
	}
	if r.ProgramFiles != nil {
		ReleaseProgramFiles = r.ProgramFiles
	}
	if r.RestartArgs != nil {
		ReleaseRestartArgs = r.RestartArgs
	}
	ReleaseTargetDir = r.TargetDir
	ReleasePackageDir = r.PackageDir
}

// ReleaseGiteeOwner Gitee 仓库属主
var ReleaseGiteeOwner string

// ReleaseGiteeRepo Gitee 仓库名
var ReleaseGiteeRepo string

// ReleaseGiteeToken Gitee 私人令牌（需 Release 读写权限）。
// 来自独立的 release.json；TUI 发布时自动预填，允许直接用配置里的令牌。
var ReleaseGiteeToken string

// ReleaseInclude 更新包包含的资源目录清单（相对 target_dir）
var ReleaseInclude []string

// ReleaseExclude 更新包打包排除规则（空时由 updatepack 使用默认清单）
var ReleaseExclude []string

// ReleaseExcludeDirsWith 含这些文件的目录整体排除（空时默认 metadata.json，即 LTPX 工具包）
var ReleaseExcludeDirsWith []string

// ReleaseBackupKeep 覆盖前备份的保留次数（nil=默认 3；0 或负数=不备份）
var ReleaseBackupKeep *int

// ReleaseProgramFiles 主程序文件清单（相对程序根目录，如 Lunar_Astral_Agents.exe）：
// 发布时纳入包，拉取时据其判定需走「关闭主程序 → 应用器覆盖 → 重启」流程
var ReleaseProgramFiles []string

// ReleaseRestartArgs 重启主程序时携带的命令行参数
var ReleaseRestartArgs []string

// ReleaseTargetDir 资源根目录（发布打包源 / 拉取解压目标；空时由 updatepack 默认 local_data）。
// 程序合并后统一取「程序根目录」（exe 所在目录）：资源位于其下 local_data/，主程序位于其根。
var ReleaseTargetDir string

// ReleasePackageDir 本地 LTPX 包根目录（空时默认 local_data/package）
var ReleasePackageDir string
