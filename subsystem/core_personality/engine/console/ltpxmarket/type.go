// Package ltpxmarket 提供基于 Gitee Release 的 LTPX 应用市场能力：
//   - 发布：本地包（local_data/package/<目录名>）→ zip 打包 → upsert Release（tag = 包 id）→ 上传附件；
//     或直接上传已打包的 .ltpx 安装包（琉璃导出格式，包内读取 metadata.json 不解压）
//   - 消费：列举全部 / 按标签筛选 / 查看描述 / 安装（下载 → 解压 → 落位到包目录）
//
// 存储约定：每个 LTPX 包对应一个 Release（单版本 upsert，市场只展示最新版），
// body 携带结构化描述（[LTPX] 标记 + ID/Tags/SHA-256 行），附件命名 ltpx_<id>.ltpx
// （.ltpx 为琉璃生态的标准安装包后缀，内容为 zip：顶层目录 = 包名）。
package ltpxmarket

// Config 应用市场配置（宿主从 lunar_config.json 的 ltpx_market 节注入；
// gitee_owner/gitee_repo 为空时宿主回退 update_pack 同名配置）
type Config struct {
	GiteeOwner string `json:"gitee_owner"` // Gitee 仓库属主
	GiteeRepo  string `json:"gitee_repo"`  // Gitee 仓库名
	// GiteeToken Gitee 私人令牌（发布必填；浏览/安装公开仓库可留空）。
	// 沿用既有安全约定：不写入配置文件，TUI 发布时手动输入注入副本
	GiteeToken string `json:"gitee_token"`
	// PackageDir LTPX 包根目录（空时默认 local_data/package，相对可执行文件目录解析）
	PackageDir string `json:"package_dir"`
}

// PackageMeta LTPX 包元数据（metadata.json，仅解析市场所需字段）
type PackageMeta struct {
	ID          string   `json:"id"`          // 包唯一标识（LTPX 广播 app_id；用作 Release tag）
	Icon        string   `json:"icon"`        // 图标文件
	Title       string   `json:"title"`       // 展示名称
	Description string   `json:"description"` // 包描述
	Tags        []string `json:"tags"`        // 包标签（如 Node-LTP / LTP9）
}

// MarketItem 市场包列表项（由 Release 解析）
type MarketItem struct {
	TagName     string   `json:"tag_name"`    // Release tag（= 包 id）
	Title       string   `json:"title"`       // 包标题
	Description string   `json:"description"` // 描述正文
	Tags        []string `json:"tags"`        // 标签
	AssetName   string   `json:"asset_name"`  // 附件文件名（ltpx_<id>.ltpx）
	AssetURL    string   `json:"asset_url"`   // 安装附件下载地址
	AssetSize   int64    `json:"asset_size"`  // 附件大小（字节）
	AssetSHA    string   `json:"asset_sha"`   // 发布时声明的 SHA-256（可空）
}

// LocalPackage 本地 LTPX 包（发布候选项）
type LocalPackage struct {
	DirName string      `json:"dir_name"` // 包目录名（zip 顶层目录名，随包保留）
	Dir     string      `json:"dir"`      // 包目录绝对路径
	Meta    PackageMeta `json:"meta"`     // metadata.json 解析结果
	Size    int64       `json:"size"`     // 目录总大小（字节）
}
