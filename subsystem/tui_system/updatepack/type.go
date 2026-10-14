// Package updatepack 提供基于 Gitee Release 的更新包发布与拉取能力
// （替代原 environment_repair 的「资源释放/补全」：资源不再嵌入程序，而是上传 Gitee 仓库）。
//
// 发布：按配置收集资源目录 → 7z 打包 → 创建 Release（tag）→ 上传附件（含 SHA-256）。
// 拉取：默认最新 Release 附件；可指定 tag；可指定 commit 哈希（优先匹配 tag，
// 未命中走 zipball 通道 —— Gitee 官方 API 支持 ref=commit SHA 下载仓库 zip）。
package updatepack

// Config 更新包功能配置（宿主从 lunar_config.json 的 update_pack 节注入）
type Config struct {
	GiteeOwner string   `json:"gitee_owner"` // Gitee 仓库属主
	GiteeRepo  string   `json:"gitee_repo"`  // Gitee 仓库名
	GiteeToken string   `json:"gitee_token"` // Gitee 私人令牌（发布必填；拉公开仓库可留空，TUI 发布时手动输入）
	Include    []string `json:"include"`     // 打包包含的资源目录（相对 target_dir）
	Exclude    []string `json:"exclude"`     // 打包排除规则（空时用默认排除清单）
	// ExcludeDirsWith 含这些文件的目录整体排除（空时默认排除含 metadata.json 的目录，
	// 即 LTPX 工具包：与 environment_repair 的资源同步规则一致）
	ExcludeDirsWith []string `json:"exclude_dirs_with"`
	// BackupKeep 覆盖前备份的保留次数（拉取时每个文件覆盖前先在
	// {target_dir}/.backup/<时间戳>/ 留存旧版本）。nil=默认 3；0 或负数=不备份
	BackupKeep *int   `json:"backup_keep"`
	TargetDir  string `json:"target_dir"` // 资源根目录（发布打包源 / 拉取解压目标，默认 local_data）
}

// Version 更新包版本信息（Release 列表项）
type Version struct {
	TagName  string `json:"tag_name"`  // 版本标签
	Name     string `json:"name"`      // 发布名称
	Body     string `json:"body"`      // 发布描述（含 SHA-256）
	AssetURL string `json:"asset_url"` // 附件下载地址
	AssetSHA string `json:"asset_sha"` // 描述中声明的 SHA-256（可空）
}

// PullKind 拉取方式
type PullKind int

const (
	// PullLatest 最新 Release
	PullLatest PullKind = iota
	// PullTag 指定 tag
	PullTag
	// PullHash 指定 commit 哈希（zipball 通道兜底）
	PullHash
)

// PullSpec 拉取参数
type PullSpec struct {
	Kind  PullKind
	Value string // tag 名或 commit 哈希（PullLatest 时为空）
}
