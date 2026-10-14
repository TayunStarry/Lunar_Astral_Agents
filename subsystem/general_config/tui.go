package GeneralConfig

// ==== TUI 控制台与更新包配置（lunar_config.json 的 tui / update_pack 节） ====
// 仅 TUI 构建消费 update_pack；桌面构建仅消费 ExitWhenFrontendsClosed。

// ExitWhenFrontendsClosed 前端页面全部关闭后是否自动退出进程。
// nil 表示未配置：桌面构建视为 true（保持「前端全关即退出」历史行为），
// TUI 构建视为 false（控制台常驻，避免关窗误退）。
var ExitWhenFrontendsClosed *bool

// UpdatePackGiteeOwner 更新包 Gitee 仓库属主
var UpdatePackGiteeOwner string

// UpdatePackGiteeRepo 更新包 Gitee 仓库名
var UpdatePackGiteeRepo string

// UpdatePackGiteeToken 更新包 Gitee 私人令牌（需 Release 读写权限）
var UpdatePackGiteeToken string

// UpdatePackInclude 打包包含的资源目录清单（相对资源根目录）
var UpdatePackInclude []string

// UpdatePackTargetDir 资源根目录（发布打包源 / 拉取解压目标；空时由 updatepack 默认 local_data）
var UpdatePackTargetDir string
