package updatepack

import (
	"fmt"

	"CorePersonality/engine/console/channel"
)

// ==== 发布通道隔离（更新包 vs LTPX 应用市场） ====
//
// 两条通道共用同一 Gitee 仓库，判别契约集中在 TuiSystem/channel 包。
// 本文件只做「Gitee 响应结构 → 通道」的映射，以及错误信息的措辞。

// assetNames 提取发布附件名列表
func assetNames(r giteeRelease) []string {
	return channel.AssetNames(r.Assets, func(a giteeAsset) string { return a.Name })
}

// releaseKind 判定发布所属通道
func releaseKind(r giteeRelease) channel.Kind {
	return channel.Classify(assetNames(r), r.Body)
}

// isUpdateRelease 是否属于更新包通道（本通道）
func isUpdateRelease(r giteeRelease) bool {
	return releaseKind(r) == channel.Update
}

// updateAssets 取更新包附件。
// 必须按前缀过滤：Gitee 会为每个 Release 自动附加源码归档（<tag>.zip/.tar.gz），
// 那不是更新包；共用仓库时还可能混入市场附件（ltpx_*.ltpx）。
func updateAssets(r giteeRelease) []giteeAsset {
	out := make([]giteeAsset, 0, len(r.Assets))
	for _, a := range r.Assets {
		if channel.IsUpdateAsset(a.Name) {
			out = append(out, a)
		}
	}
	return out
}

// channelMismatchError 通道不符错误：
// 共用仓库后「tag 撞到另一条通道」是可预期的操作失误，错误信息必须把
// 因由与出路讲清楚，否则会被误当成网络故障反复重试。
func channelMismatchError(tag string, got channel.Kind) error {
	return fmt.Errorf("Release %s 属于「%s」通道，不是更新包；"+
		"更新包与 LTPX 应用市场共用同一仓库，请确认 tag，或在 TUI 的「LTPX 应用市场」中操作", tag, got)
}
