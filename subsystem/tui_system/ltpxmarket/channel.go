package ltpxmarket

import (
	"fmt"

	"LunarSubsystem/TuiSystem/channel"
)

// ==== 发布通道隔离（LTPX 应用市场 vs 更新包） ====
//
// 市场与更新包共用同一 Gitee 仓库，判别契约集中在 TuiSystem/channel 包。
// 本文件只做「Gitee 响应结构 → 通道」的映射。

// assetNames 提取发布附件名列表
func assetNames(r giteeRelease) []string {
	return channel.AssetNames(r.Assets, func(a giteeAsset) string { return a.Name })
}

// releaseKind 判定发布所属通道
func releaseKind(r giteeRelease) channel.Kind {
	return channel.Classify(assetNames(r), r.Body)
}

// channelMismatchError 通道不符错误（共用仓库后 tag 撞车是可预期的操作失误）
func channelMismatchError(tag string, got channel.Kind) error {
	return fmt.Errorf("Release %s 属于「%s」通道，不是 LTPX 应用市场包；"+
		"两条通道共用同一仓库，请更换包 id，或到「资源更新 管理」中操作", tag, got)
}
