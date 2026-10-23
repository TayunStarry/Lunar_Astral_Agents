package config

// ==== Gitee Release 发布通道（lunar_config.json 的 release 节） ====
//
// 更新包与 LTPX 应用市场共用同一仓库：两者都以 Gitee Release 存储，
// 在仓库内以「描述标记 + 附件名前缀」严格区分——
//
//	更新包：[UPDATE] 标记 / lunar_updatepack_*.7z.001
//	市场包：[LTPX]   标记 / ltpx_<id>.ltpx
//
// 因此配置只需一套仓库信息；具体通道隔离由 tui_system/updatepack 与
// tui_system/ltpxmarket 各自实现（互不抓取对方的发布）。

// ReleaseGiteeOwner Gitee 仓库属主
var ReleaseGiteeOwner string

// ReleaseGiteeRepo Gitee 仓库名
var ReleaseGiteeRepo string

// ReleaseGiteeToken Gitee 私人令牌（需 Release 读写权限）。
// 通常留空：发布时在 TUI 中手动输入，令牌不落盘。
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
