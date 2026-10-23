// Package channel 定义 Gitee Release「发布通道」的划分与判别契约。
//
// 更新包与 LTPX 应用市场**共用同一仓库**（同一批 Release），因此每条发布都必须
// 能判定归属，否则会出现「拉更新包拉到 LTPX 工具包」「版本列表混进市场包」这类串台。
// 判别依据两条，互为兜底：
//
//  1. 描述标记：更新包 body 含 [UPDATE]；市场包 body 含 [LTPX]
//  2. 附件名前缀：更新包 lunar_updatepack_*；市场包 ltpx_*.ltpx
//
// 只看附件会漏掉「发布已建、附件未传完」的空发布；只看标记会漏掉隔离上线前
// 发布的历史更新包（当年没有 [UPDATE] 标记，但附件前缀一直在）。
//
// 这里刻意只接受「附件名列表 + 描述文本」两个纯参数，不依赖任何 Gitee 响应结构：
// 契约集中在一处，两个子包各自映射即可，避免前缀/标记两边各写一份而日久漂移。
package channel

import "strings"

// Kind 发布通道
type Kind int

const (
	// Unknown 无法判定的发布（既无通道标记，也无任一通道的附件）
	Unknown Kind = iota
	// Update 更新包通道（资源/主程序增量包）
	Update
	// Market LTPX 应用市场通道（工具包）
	Market
)

// String 通道中文名（用于日志与错误信息）
func (k Kind) String() string {
	switch k {
	case Update:
		return "更新包"
	case Market:
		return "LTPX 应用市场"
	default:
		return "未识别"
	}
}

// 通道判定常量（发布端与消费端共用，改动即两侧同时生效）
const (
	// UpdateAssetPrefix 更新包附件名前缀（打包产物 lunar_updatepack_<version>.7z.001）
	UpdateAssetPrefix = "lunar_updatepack_"
	// UpdateMarker 更新包 Release 描述标记（body 中出现即视为更新包）
	UpdateMarker = "[UPDATE]"
	// MarketAssetPrefix 市场附件名前缀（ltpx_<id>.ltpx）
	MarketAssetPrefix = "ltpx_"
	// MarketMarker 市场 Release 描述标记
	MarketMarker = "[LTPX]"
)

// Classify 判定一条发布所属通道。
// assetNames 为该发布的全部附件名，body 为描述正文；两者任一命中即可判定。
func Classify(assetNames []string, body string) Kind {
	hasUpdateAsset, hasMarketAsset := false, false
	for _, name := range assetNames {
		switch {
		case strings.HasPrefix(name, UpdateAssetPrefix):
			hasUpdateAsset = true
		case strings.HasPrefix(name, MarketAssetPrefix):
			hasMarketAsset = true
		}
	}
	// 标记大小写不敏感：Gitee 描述为人工/程序写入，避免因大小写漏判
	upper := strings.ToUpper(body)
	switch {
	// 附件证据优先于标记：同时命中时以附件为准（附件事务性更强，不会误写）
	case hasUpdateAsset:
		return Update
	case hasMarketAsset:
		return Market
	case strings.Contains(upper, UpdateMarker):
		return Update
	case strings.Contains(upper, MarketMarker):
		return Market
	}
	return Unknown
}

// IsUpdateAsset 附件名是否属于更新包通道
func IsUpdateAsset(name string) bool { return strings.HasPrefix(name, UpdateAssetPrefix) }

// IsMarketAsset 附件名是否属于市场通道
func IsMarketAsset(name string) bool { return strings.HasPrefix(name, MarketAssetPrefix) }

// AssetNames 提取附件名列表（调用方各自映射自己的响应结构）
func AssetNames[T any](assets []T, name func(T) string) []string {
	out := make([]string, 0, len(assets))
	for _, a := range assets {
		out = append(out, name(a))
	}
	return out
}
